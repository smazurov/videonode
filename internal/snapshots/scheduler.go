package snapshots

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// subscriber receives JPEG entries on the channel. Slow subscribers see
// frames dropped (channel is buffered, non-blocking send).
type subscriber struct {
	fps int
	ch  chan *Entry
}

// subscriberSet groups all active subscribers for a {kind,id} preview
// stream. The scheduler goroutine reads `subs` under `mu` to decide the
// poll rate; closes when subs becomes empty. `gen` lets stale schedulers
// (left over after a teardown race) detect they've been superseded.
type subscriberSet struct {
	mu       sync.Mutex
	subs     map[*subscriber]struct{}
	gen      uint64
	stopOnce sync.Once
	stop     chan struct{}
}

// SubscriptionHandle is returned by Subscribe; clients read from Frames()
// and call Close() when they're done (or when the request context ends).
type SubscriptionHandle struct {
	cache  *Cache
	kind   Kind
	id     string
	set    *subscriberSet
	sub    *subscriber
	closed atomic.Bool
}

// Frames returns the channel new entries are delivered on. Closed when
// Close() is invoked.
func (h *SubscriptionHandle) Frames() <-chan *Entry { return h.sub.ch }

// Close detaches the subscriber. Idempotent.
func (h *SubscriptionHandle) Close() {
	if h.closed.Swap(true) {
		return
	}
	h.cache.unsubscribe(h.kind, h.id, h.set, h.sub)
}

// Subscribe registers a subscriber at `fps` Hz for {kind,id}. The
// returned handle's Frames() yields the latest cached Entry every refresh
// tick the scheduler runs. `fps` is clamped to [1, cfg.MaxFPS]; pass 0
// to use cfg.DefaultFPS.
func (c *Cache) Subscribe(kind Kind, id string, fps int) *SubscriptionHandle {
	fps = c.cfg.ClampFPS(fps)
	sub := &subscriber{fps: fps, ch: make(chan *Entry, 2)}

	key := cacheKey(kind, id)
	c.mu.Lock()
	set, ok := c.subs[key]
	if !ok {
		set = &subscriberSet{subs: make(map[*subscriber]struct{}), stop: make(chan struct{})}
		c.subs[key] = set
	}
	c.mu.Unlock()

	set.mu.Lock()
	set.subs[sub] = struct{}{}
	if len(set.subs) == 1 {
		// First subscriber — launch the scheduler.
		set.gen++
		go c.runScheduler(kind, id, set, set.gen)
	}
	set.mu.Unlock()

	return &SubscriptionHandle{cache: c, kind: kind, id: id, set: set, sub: sub}
}

func (c *Cache) unsubscribe(kind Kind, id string, set *subscriberSet, sub *subscriber) {
	set.mu.Lock()
	delete(set.subs, sub)
	last := len(set.subs) == 0
	if last {
		set.stopOnce.Do(func() { close(set.stop) })
	}
	set.mu.Unlock()

	if last {
		// Drop the set entry under the cache mutex so a new subscriber
		// starts a fresh scheduler.
		c.mu.Lock()
		if cur, ok := c.subs[cacheKey(kind, id)]; ok && cur == set {
			delete(c.subs, cacheKey(kind, id))
		}
		c.mu.Unlock()
	}

	// Closing after removal so any concurrent broadcast won't panic on
	// send. (We send under set.mu, so by the time we get here all sends
	// for this sub are done.)
	close(sub.ch)
}

// runScheduler is the per-{kind,id} refresh loop. It ticks at the
// highest fps any current subscriber asked for; refreshes the cache;
// fans the latest Entry out to every subscriber via a non-blocking send.
func (c *Cache) runScheduler(kind Kind, id string, set *subscriberSet, gen uint64) {
	logger := logging.GetLogger("snapshots")
	logger.Debug("Scheduler starting", "kind", string(kind), "id", id, "gen", gen)
	defer logger.Debug("Scheduler exiting", "kind", string(kind), "id", id, "gen", gen)

	for {
		fps := set.peakFPS()
		if fps <= 0 {
			return
		}
		interval := time.Second / time.Duration(fps)

		// Refresh — bypass the staleness check so the scheduler can
		// drive at the subscriber's chosen fps. Still coalesces with
		// concurrent one-shot HTTP requests through inflight.
		ctx, cancel := context.WithTimeout(context.Background(), c.cfg.RPCTimeout+500*time.Millisecond)
		entry, err := c.Refresh(ctx, kind, id)
		cancel()
		if err != nil {
			logger.Debug("Scheduler refresh failed", "kind", string(kind), "id", id, "error", err)
			// Fall through to the wait; next tick will retry.
		} else {
			set.broadcast(entry)
		}

		select {
		case <-set.stop:
			return
		case <-time.After(interval):
		}

		// If a higher-priority subscriber dropped, fps may have changed;
		// peakFPS re-reads it on the next iteration. If the set was torn
		// down and re-created mid-loop (rapid resubscribe with same key),
		// `gen` mismatches and we exit so the new scheduler can take over.
		set.mu.Lock()
		curGen := set.gen
		set.mu.Unlock()
		if curGen != gen {
			return
		}
	}
}

// peakFPS returns the highest fps any current subscriber asked for, or 0
// if there are no subscribers.
func (s *subscriberSet) peakFPS() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	best := 0
	for sub := range s.subs {
		if sub.fps > best {
			best = sub.fps
		}
	}
	return best
}

// broadcast sends `entry` to every subscriber non-blockingly. A slow
// subscriber (channel full) just misses this frame.
func (s *subscriberSet) broadcast(entry *Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sub := range s.subs {
		select {
		case sub.ch <- entry:
		default:
		}
	}
}

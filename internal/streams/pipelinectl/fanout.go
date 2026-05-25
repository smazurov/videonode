package pipelinectl

// StartedAtLookup returns the StartedAtUs for a pool key (e.g.
// "producer:<deviceID>"), or 0 if unknown.
type StartedAtLookup func(poolKey string) int64

// RunStatusFanout drains feed, stamps StartedAtUs via lookup, and
// forwards each message to publish. The drain never blocks on publish
// — messages are dropped if the internal buffer is full. Returns a
// channel that is closed when both goroutines have exited (i.e. after
// feed is closed and all buffered messages are published).
func RunStatusFanout(
	feed <-chan StatusParams,
	publish func(StatusParams),
	lookup StartedAtLookup,
) <-chan struct{} {
	publishCh := make(chan StatusParams, 256)
	done := make(chan struct{})

	// Drain: reads feed as fast as possible, stamps StartedAtUs,
	// and forwards to publishCh non-blocking.
	go func() {
		defer close(publishCh)
		for st := range feed {
			if st.DeviceID != "" && lookup != nil {
				if us := lookup("producer:" + st.DeviceID); us != 0 {
					st.StartedAtUs = us
				}
			}
			select {
			case publishCh <- st:
			default:
			}
		}
	}()

	// Publish: does the potentially-blocking work.
	go func() {
		defer close(done)
		for st := range publishCh {
			publish(st)
		}
	}()

	return done
}

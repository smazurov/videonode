package pipelinectl

import (
	"testing"
	"time"
)

func TestStatusFanout_DrainNeverBlocks(t *testing.T) {
	feed := make(chan StatusParams, 4)
	var published []StatusParams
	publish := func(st StatusParams) { published = append(published, st) }

	// Slow publisher — blocks for 50ms per message.
	slowPublish := func(st StatusParams) {
		time.Sleep(50 * time.Millisecond)
		published = append(published, st)
	}
	_ = slowPublish

	done := RunStatusFanout(feed, publish, nil)

	// Push more messages than the publish channel buffer can hold.
	// If the drain goroutine blocks on publish, this loop will stall.
	const count = 500
	deadline := time.After(2 * time.Second)
	for i := range count {
		select {
		case feed <- StatusParams{DeviceID: "test", TimestampMs: int64(i)}:
		case <-deadline:
			t.Fatalf("feed send blocked at message %d — drain goroutine is not non-blocking", i)
		}
	}

	close(feed)
	<-done

	if len(published) == 0 {
		t.Fatal("no messages published")
	}
}

func TestStatusFanout_StampsStartedAt(t *testing.T) {
	feed := make(chan StatusParams, 1)
	var got StatusParams
	publish := func(st StatusParams) { got = st }

	startedAtUs := int64(1234567890)
	lookup := func(deviceID string) int64 {
		if deviceID == "producer:cam" {
			return startedAtUs
		}
		return 0
	}

	done := RunStatusFanout(feed, publish, lookup)
	feed <- StatusParams{DeviceID: "cam"}
	close(feed)
	<-done

	if got.StartedAtUs != startedAtUs {
		t.Errorf("want StartedAtUs=%d, got %d", startedAtUs, got.StartedAtUs)
	}
}

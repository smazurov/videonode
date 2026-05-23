package pipeline

import (
	"sort"
	"testing"
)

func sortedStrings(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestProducerRegistry_FirstReconcileStartsAll(t *testing.T) {
	r := NewProducerRegistry()
	d := r.Reconcile("streamA", []string{"hdmi0", "usb-1-2"})

	if got := sortedStrings(d.ToStart); !equal(got, []string{"hdmi0", "usb-1-2"}) {
		t.Errorf("ToStart = %v, want [hdmi0 usb-1-2]", got)
	}
	if len(d.ToStop) != 0 {
		t.Errorf("ToStop = %v, want []", d.ToStop)
	}
	if r.Refcount("hdmi0") != 1 || r.Refcount("usb-1-2") != 1 {
		t.Errorf("refcounts: hdmi0=%d usb-1-2=%d, want 1 each",
			r.Refcount("hdmi0"), r.Refcount("usb-1-2"))
	}
}

func TestProducerRegistry_SharedDeviceNoSecondStart(t *testing.T) {
	r := NewProducerRegistry()
	r.Reconcile("streamA", []string{"hdmi0"})
	d := r.Reconcile("streamB", []string{"hdmi0"})

	if len(d.ToStart) != 0 {
		t.Errorf("streamB reuse of hdmi0 should not start: ToStart=%v", d.ToStart)
	}
	if r.Refcount("hdmi0") != 2 {
		t.Errorf("refcount hdmi0 = %d, want 2", r.Refcount("hdmi0"))
	}
}

func TestProducerRegistry_DropToZeroStops(t *testing.T) {
	r := NewProducerRegistry()
	r.Reconcile("streamA", []string{"hdmi0"})
	r.Reconcile("streamB", []string{"hdmi0"})
	d := r.Reconcile("streamA", nil)

	if len(d.ToStop) != 0 {
		t.Errorf("streamA release with streamB still holding should not stop: ToStop=%v",
			d.ToStop)
	}
	d = r.Reconcile("streamB", nil)
	if got := sortedStrings(d.ToStop); !equal(got, []string{"hdmi0"}) {
		t.Errorf("streamB final release: ToStop=%v, want [hdmi0]", got)
	}
	if r.Refcount("hdmi0") != 0 {
		t.Errorf("refcount hdmi0 = %d after both released, want 0", r.Refcount("hdmi0"))
	}
}

func TestProducerRegistry_IdempotentReconcile(t *testing.T) {
	r := NewProducerRegistry()
	r.Reconcile("streamA", []string{"hdmi0"})
	d := r.Reconcile("streamA", []string{"hdmi0"})
	if len(d.ToStart) != 0 || len(d.ToStop) != 0 {
		t.Errorf("idempotent reconcile produced delta: ToStart=%v ToStop=%v",
			d.ToStart, d.ToStop)
	}
}

func TestProducerRegistry_ReleaseAll(t *testing.T) {
	r := NewProducerRegistry()
	r.Reconcile("streamA", []string{"hdmi0", "usb-1-2"})
	r.Reconcile("streamB", []string{"usb-1-2"})
	d := r.ReleaseAll("streamA")

	if got := sortedStrings(d.ToStop); !equal(got, []string{"hdmi0"}) {
		t.Errorf("ReleaseAll streamA: ToStop=%v, want [hdmi0] (usb-1-2 still held by streamB)",
			got)
	}
	if r.Refcount("usb-1-2") != 1 {
		t.Errorf("usb-1-2 refcount after streamA release = %d, want 1",
			r.Refcount("usb-1-2"))
	}
}

func TestProducerRegistry_DeltaOnDeviceSwap(t *testing.T) {
	r := NewProducerRegistry()
	r.Reconcile("streamA", []string{"hdmi0"})
	d := r.Reconcile("streamA", []string{"hdmi1"})

	if got := sortedStrings(d.ToStart); !equal(got, []string{"hdmi1"}) {
		t.Errorf("swap: ToStart=%v, want [hdmi1]", got)
	}
	if got := sortedStrings(d.ToStop); !equal(got, []string{"hdmi0"}) {
		t.Errorf("swap: ToStop=%v, want [hdmi0]", got)
	}
}

func TestProducerRegistry_ConsumersOf(t *testing.T) {
	r := NewProducerRegistry()
	r.Reconcile("streamA", []string{"hdmi0"})
	r.Reconcile("streamB", []string{"hdmi0"})
	got := sortedStrings(r.ConsumersOf("hdmi0"))
	if !equal(got, []string{"streamA", "streamB"}) {
		t.Errorf("ConsumersOf(hdmi0) = %v, want [streamA streamB]", got)
	}
	if r.ConsumersOf("unknown") != nil {
		t.Errorf("ConsumersOf(unknown) should be nil, got %v", r.ConsumersOf("unknown"))
	}
}

func TestProducerRegistry_EmptyConsumerID(t *testing.T) {
	r := NewProducerRegistry()
	d := r.Reconcile("", []string{"hdmi0"})
	if len(d.ToStart) != 0 || len(d.ToStop) != 0 {
		t.Errorf("empty consumerID should no-op: %+v", d)
	}
	if r.Refcount("hdmi0") != 0 {
		t.Errorf("empty consumerID should not claim device, refcount=%d", r.Refcount("hdmi0"))
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

package pipeline

import (
	"testing"
)

func TestSourceRegistry_PutAndGet(t *testing.T) {
	r := NewSourceRegistry()
	s := Source{ID: "hdmi0", Device: "/dev/video0"}
	if _, existed := r.Put(s); existed {
		t.Fatalf("first Put should report existed=false")
	}
	got, ok := r.Get("hdmi0")
	if !ok || got.ID != "hdmi0" || got.Device != "/dev/video0" {
		t.Fatalf("Get returned %+v ok=%v", got, ok)
	}
	// Update overwrites.
	s2 := Source{ID: "hdmi0", TestMode: true}
	prior, existed := r.Put(s2)
	if !existed {
		t.Fatalf("second Put should report existed=true")
	}
	if prior.Device != "/dev/video0" {
		t.Errorf("prior.Device = %q, want /dev/video0", prior.Device)
	}
}

func TestSourceRegistry_DeleteAndIDs(t *testing.T) {
	r := NewSourceRegistry()
	r.Put(Source{ID: "b"})
	r.Put(Source{ID: "a"})
	r.Put(Source{ID: "c"})

	ids := r.IDs()
	if !equal(ids, []string{"a", "b", "c"}) {
		t.Errorf("IDs = %v, want [a b c]", ids)
	}
	if _, ok := r.Delete("b"); !ok {
		t.Error("Delete(b) should return ok=true")
	}
	if _, ok := r.Get("b"); ok {
		t.Error("Get after Delete should return ok=false")
	}
	if _, ok := r.Delete("missing"); ok {
		t.Error("Delete(missing) should return ok=false")
	}
}

func TestComposerRegistry_PutAndGet(t *testing.T) {
	r := NewComposerRegistry()
	c := Composer{ID: "main", Canvas: CanvasDims{W: 1920, H: 1080}}
	if _, existed := r.Put(c); existed {
		t.Fatalf("first Put should report existed=false")
	}
	got, ok := r.Get("main")
	if !ok || got.Canvas.W != 1920 || got.Canvas.H != 1080 {
		t.Fatalf("Get returned %+v ok=%v", got, ok)
	}
}

func TestComposerRegistry_DeleteAndIDs(t *testing.T) {
	r := NewComposerRegistry()
	r.Put(Composer{ID: "z"})
	r.Put(Composer{ID: "a"})

	ids := r.IDs()
	if !equal(ids, []string{"a", "z"}) {
		t.Errorf("IDs = %v, want [a z]", ids)
	}
	if _, ok := r.Delete("z"); !ok {
		t.Error("Delete(z) should return ok=true")
	}
	if _, ok := r.Delete("missing"); ok {
		t.Error("Delete(missing) should return ok=false")
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

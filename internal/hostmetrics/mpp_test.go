package hostmetrics

import (
	"bytes"
	"os"
	"testing"
)

// fixtures captured live from /proc/mpp_service/load on an RK3588 rig
// (kernel 6.1.115-vendor-rk35xx) with one active h264 rkvenc session.

func TestParseMPPLoad_Enabled(t *testing.T) {
	data, err := os.ReadFile("testdata/mpp_load_enabled.txt")
	if err != nil {
		t.Fatal(err)
	}
	cores, err := parseMPPLoad(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cores) != 14 {
		t.Fatalf("got %d cores, want 14", len(cores))
	}

	byNode := make(map[string]RKMPPCore, len(cores))
	for _, c := range cores {
		byNode[c.Node] = c
	}

	enc := byNode["fdbd0000.rkvenc-core"]
	if enc.Class != "rkvenc" {
		t.Errorf("rkvenc class = %q, want %q", enc.Class, "rkvenc")
	}
	if enc.LoadPercent != 22.18 {
		t.Errorf("rkvenc load = %v, want 22.18", enc.LoadPercent)
	}
	if enc.UtilPercent != 21.63 {
		t.Errorf("rkvenc utilization = %v, want 21.63", enc.UtilPercent)
	}

	classCases := map[string]string{
		"fdbe0000.rkvenc-core": "rkvenc",
		"fdc38100.rkvdec-core": "rkvdec",
		"fdba0000.jpege-core":  "jpege",
		"fdb51000.avsd-plus":   "avsd-plus",
		"fdb50400.vdpu":        "vdpu",
		"fdc70000.av1d":        "av1d",
	}
	for node, want := range classCases {
		if got := byNode[node].Class; got != want {
			t.Errorf("class(%s) = %q, want %q", node, got, want)
		}
	}
}

func TestParseMPPLoad_Disabled(t *testing.T) {
	data, err := os.ReadFile("testdata/mpp_load_disabled.txt")
	if err != nil {
		t.Fatal(err)
	}
	cores, err := parseMPPLoad(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cores) != 0 {
		t.Errorf("got %d cores from disabled banner, want 0", len(cores))
	}
}

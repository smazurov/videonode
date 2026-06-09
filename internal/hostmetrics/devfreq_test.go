package hostmetrics

import "testing"

func TestParseDevfreqLoad(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantLoad float64
		wantFreq int64
		wantOK   bool
	}{
		{"gpu idle", "0@200000000Hz", 0, 200000000, true},
		{"gpu busy", "22@600000000Hz", 22, 600000000, true},
		{"npu max", "100@1000000000Hz", 100, 1000000000, true},
		{"empty", "", 0, 0, false},
		{"no at", "42", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			load, freq, ok := parseDevfreqLoad(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if load != tt.wantLoad {
				t.Errorf("load = %v, want %v", load, tt.wantLoad)
			}
			if freq != tt.wantFreq {
				t.Errorf("freq = %v, want %v", freq, tt.wantFreq)
			}
		})
	}
}

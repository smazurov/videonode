package collectors

import (
	"strings"
	"testing"
)

func TestMPPCollectorParseLine(t *testing.T) {
	m := &MPPCollector{}

	tests := []struct {
		name       string
		line       string
		wantDevice string
		wantLoad   float64
		wantUtil   float64
		wantErr    bool
	}{
		{
			name:       "valid line",
			line:       "rkvenc: load: 45% utilization: 78%",
			wantDevice: "rkvenc:",
			wantLoad:   45,
			wantUtil:   78,
		},
		{
			name:       "valid line with decimals",
			line:       "rkvdec: load: 12.5% utilization: 33.3%",
			wantDevice: "rkvdec:",
			wantLoad:   12.5,
			wantUtil:   33.3,
		},
		{
			name:    "insufficient fields",
			line:    "rkvenc: load:",
			wantErr: true,
		},
		{
			name:    "missing load",
			line:    "rkvenc: utilization: 78%",
			wantErr: true,
		},
		{
			name:    "missing utilization",
			line:    "rkvenc: load: 45%",
			wantErr: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, err := m.parseLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if device.Name != tt.wantDevice {
				t.Errorf("Name = %q, want %q", device.Name, tt.wantDevice)
			}
			if device.Load != tt.wantLoad {
				t.Errorf("Load = %v, want %v", device.Load, tt.wantLoad)
			}
			if device.Utilization != tt.wantUtil {
				t.Errorf("Utilization = %v, want %v", device.Utilization, tt.wantUtil)
			}
		})
	}
}

func TestMPPCollectorParseContent(t *testing.T) {
	m := &MPPCollector{}

	content := `rkvenc: load: 45% utilization: 78%
rkvdec: load: 12% utilization: 33%

invalid line here
rkvenc_1: load: 0% utilization: 0%`

	devices, err := m.parseContent(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(devices) != 3 {
		t.Fatalf("got %d devices, want 3", len(devices))
	}

	expected := []struct {
		name string
		load float64
		util float64
	}{
		{"rkvenc:", 45, 78},
		{"rkvdec:", 12, 33},
		{"rkvenc_1:", 0, 0},
	}

	for i, exp := range expected {
		if devices[i].Name != exp.name {
			t.Errorf("device[%d].Name = %q, want %q", i, devices[i].Name, exp.name)
		}
		if devices[i].Load != exp.load {
			t.Errorf("device[%d].Load = %v, want %v", i, devices[i].Load, exp.load)
		}
		if devices[i].Utilization != exp.util {
			t.Errorf("device[%d].Utilization = %v, want %v", i, devices[i].Utilization, exp.util)
		}
	}
}

func TestMPPCollectorEmptyContent(t *testing.T) {
	m := &MPPCollector{}

	devices, err := m.parseContent(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("got %d devices, want 0", len(devices))
	}
}

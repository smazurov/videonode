package metrics

import "testing"

func TestProducerMetricsViaGatherer(t *testing.T) {
	tests := []struct {
		name       string
		sourceID   string
		rssBytes   float64
		cpuPercent float64
	}{
		{"hdmi source", "hdmi-slides", 128 * 1024 * 1024, 12.5},
		{"usb source", "cam-host", 42 * 1024 * 1024, 7.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DeleteProducerMetrics(tt.sourceID)
			defer DeleteProducerMetrics(tt.sourceID)

			SetProducerRSS(tt.sourceID, tt.rssBytes)
			SetProducerCPU(tt.sourceID, tt.cpuPercent)

			all, err := GetProducerMetricsFromRegistry()
			if err != nil {
				t.Fatalf("gatherer: %v", err)
			}
			got := all[tt.sourceID]
			if got == nil {
				t.Fatalf("no metrics for source %q", tt.sourceID)
			}
			if got.RSSBytes != tt.rssBytes {
				t.Errorf("RSSBytes = %v, want %v", got.RSSBytes, tt.rssBytes)
			}
			if got.CPUPercent != tt.cpuPercent {
				t.Errorf("CPUPercent = %v, want %v", got.CPUPercent, tt.cpuPercent)
			}
		})
	}
}

func TestProducerMetrics_DeleteClears(t *testing.T) {
	sourceID := "tmp-source"
	SetProducerRSS(sourceID, 1000)
	SetProducerCPU(sourceID, 99.0)

	DeleteProducerMetrics(sourceID)

	all, err := GetProducerMetricsFromRegistry()
	if err != nil {
		t.Fatalf("gatherer: %v", err)
	}
	if _, ok := all[sourceID]; ok {
		t.Errorf("metrics for %q should be cleared", sourceID)
	}
}

func TestProducerMetrics_IsolatedPerSource(t *testing.T) {
	DeleteProducerMetrics("src-a")
	DeleteProducerMetrics("src-b")
	defer DeleteProducerMetrics("src-a")
	defer DeleteProducerMetrics("src-b")

	SetProducerRSS("src-a", 100)
	SetProducerRSS("src-b", 200)

	all, err := GetProducerMetricsFromRegistry()
	if err != nil {
		t.Fatalf("gatherer: %v", err)
	}
	if all["src-a"] == nil || all["src-a"].RSSBytes != 100 {
		t.Errorf("src-a RSS = %+v, want 100", all["src-a"])
	}
	if all["src-b"] == nil || all["src-b"].RSSBytes != 200 {
		t.Errorf("src-b RSS = %+v, want 200", all["src-b"])
	}
}

package hostmetrics

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const devfreqGlob = "/sys/class/devfreq/"

// DevfreqLoad is a device-global utilization sample from a Linux devfreq node.
// The Mali GPU and the RKNN NPU both expose the same shape: a `load` file
// reading "<load>@<freq>Hz". Used for both — the Node field disambiguates.
type DevfreqLoad struct {
	Node        string  `json:"node" doc:"devfreq node, e.g. fb000000.gpu-panthor / fdab0000.npu"`
	LoadPercent float64 `json:"load_percent" doc:"Device busy percentage (devfreq governor)"`
	CurFreqHz   int64   `json:"cur_freq_hz,omitempty" doc:"Current operating frequency in Hz"`
	MaxFreqHz   int64   `json:"max_freq_hz,omitempty" doc:"Maximum operating frequency in Hz"`
}

// readDevfreqLoad returns the load of the first devfreq node whose directory
// name contains match (e.g. "gpu", "npu"). Returns nil when no such node
// exists — the caller treats that as "hardware absent".
func readDevfreqLoad(match string) *DevfreqLoad {
	entries, err := os.ReadDir(devfreqGlob)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.Contains(name, match) {
			continue
		}
		dir := filepath.Join(devfreqGlob, name)
		load, freq, ok := parseDevfreqLoad(readTrim(filepath.Join(dir, "load")))
		if !ok {
			continue
		}
		d := &DevfreqLoad{Node: name, LoadPercent: load, CurFreqHz: freq}
		if maxFreq, err := strconv.ParseInt(readTrim(filepath.Join(dir, "max_freq")), 10, 64); err == nil {
			d.MaxFreqHz = maxFreq
		}
		return d
	}
	return nil
}

// parseDevfreqLoad parses the rockchip devfreq `load` format "<load>@<freq>Hz",
// e.g. "22@200000000Hz". The load is a percentage (0-100); the frequency the
// reading was taken at follows the '@'.
func parseDevfreqLoad(s string) (load float64, freqHz int64, ok bool) {
	loadStr, freqPart, ok := strings.Cut(s, "@")
	if !ok {
		return 0, 0, false
	}
	load, err := strconv.ParseFloat(strings.TrimSpace(loadStr), 64)
	if err != nil {
		return 0, 0, false
	}
	freqStr := strings.TrimSuffix(strings.TrimSpace(freqPart), "Hz")
	freqHz, _ = strconv.ParseInt(freqStr, 10, 64)
	return load, freqHz, true
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

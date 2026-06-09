// Package hostmetrics reads device-global hardware utilization from the host
// kernel interfaces — the Rockchip MPP codec pool (/proc/mpp_service/load) and
// the Mali GPU / RKNN NPU devfreq nodes (/sys/class/devfreq). All readers are
// presence-gated: a missing node yields nil/empty, never an error, so the
// daemon runs unchanged on hosts without the hardware.
package hostmetrics

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

const mppLoadPath = "/proc/mpp_service/load"

// RKMPPCore is one Rockchip MPP hardware IP core's busy figures, parsed from a
// line of /proc/mpp_service/load. The kernel reports per-device-tree node (per
// core), e.g. the two rkvenc cores report independently.
type RKMPPCore struct {
	Node        string  `json:"node" doc:"Device-tree node, e.g. fdbd0000.rkvenc-core"`
	Class       string  `json:"class" doc:"RKMPP IP class: rkvenc/rkvdec/jpege/jpegd/vepu/vdpu/iep/av1d/avsd-plus"`
	LoadPercent float64 `json:"load_percent" doc:"Percent of the sampling window the core was busy"`
	UtilPercent float64 `json:"utilization_percent" doc:"Percent of active time spent in hardware execution"`
}

// readRKMPPCores parses /proc/mpp_service/load into per-core busy figures.
// Returns an empty slice (no error) when the node is absent or when load
// sampling is disabled (load_interval == 0), so callers can poll
// unconditionally. The kernel emits a "please set load_interval first" banner
// instead of data when sampling is off — those lines carry no load token and
// are skipped.
func readRKMPPCores() ([]RKMPPCore, error) {
	f, err := os.Open(mppLoadPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	return parseMPPLoad(f)
}

func parseMPPLoad(r io.Reader) ([]RKMPPCore, error) {
	var cores []RKMPPCore
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		load, okLoad := percentAfter(fields, "load:")
		util, okUtil := percentAfter(fields, "utilization:")
		if !okLoad || !okUtil {
			continue
		}
		cores = append(cores, RKMPPCore{
			Node:        fields[0],
			Class:       mppClass(fields[0]),
			LoadPercent: load,
			UtilPercent: util,
		})
	}
	return cores, scanner.Err()
}

// percentAfter finds the field following the token and parses it as a
// percentage (trailing '%' stripped).
func percentAfter(fields []string, token string) (float64, bool) {
	for i, f := range fields {
		if f == token && i+1 < len(fields) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(fields[i+1], "%"), 64)
			if err != nil {
				return 0, false
			}
			return v, true
		}
	}
	return 0, false
}

// mppClass reduces a device-tree node to its IP class for grouping in the UI:
// "fdbd0000.rkvenc-core" -> "rkvenc", "fdba0000.jpege-core" -> "jpege",
// "fdb51000.avsd-plus" -> "avsd-plus".
func mppClass(node string) string {
	name := node
	if _, after, ok := strings.Cut(node, "."); ok {
		name = after
	}
	return strings.TrimSuffix(name, "-core")
}

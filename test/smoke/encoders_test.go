//go:build smoke

package smoke

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// detectExpectedEncoderFamily inspects the host and returns one of:
//   - "mpp"      — Rockchip SBC (Orange Pi, Radxa, etc.)
//   - "nvenc"    — NVIDIA GPU present
//   - "vaapi"    — Intel/AMD GPU with VAAPI driver present
//   - "software" — no hardware acceleration detected
func detectExpectedEncoderFamily() string {
	// Rockchip SBC check — /proc/device-tree/compatible is a NUL-separated
	// list on devicetree platforms.
	if data, err := os.ReadFile("/proc/device-tree/compatible"); err == nil {
		if bytes.Contains(bytes.ToLower(data), []byte("rockchip")) {
			return "mpp"
		}
	}

	// NVIDIA check — fastest via /proc/driver/nvidia or lspci.
	if _, err := os.Stat("/proc/driver/nvidia"); err == nil {
		return "nvenc"
	}
	if path, err := exec.LookPath("lspci"); err == nil {
		out, err := exec.Command(path).CombinedOutput()
		if err == nil && bytes.Contains(bytes.ToLower(out), []byte("nvidia")) {
			return "nvenc"
		}
	}

	// VAAPI check — Intel/AMD render node.
	if matches, _ := filepath.Glob("/dev/dri/renderD*"); len(matches) > 0 {
		return "vaapi"
	}

	return "software"
}

// expectedEncoderPrefixes maps each detected family to the encoder names that
// should appear in [validation.h264].working or [validation.h265].working.
func expectedEncoderPrefixes(family string) []string {
	switch family {
	case "mpp":
		return []string{"h264_rkmpp", "hevc_rkmpp"}
	case "nvenc":
		return []string{"h264_nvenc", "hevc_nvenc"}
	case "vaapi":
		return []string{"h264_vaapi", "hevc_vaapi"}
	default:
		return []string{"libx264", "libx265"}
	}
}

type validationFile struct {
	Validation struct {
		H264 struct {
			Working []string `toml:"working"`
		} `toml:"h264"`
		H265 struct {
			Working []string `toml:"working"`
		} `toml:"h265"`
	} `toml:"validation"`
}

func TestEncoderFamily(t *testing.T) {
	want := expectedEncoderPrefixes(expectedEncoderFamily)
	t.Logf("expected encoder family: %s (want any of: %s)",
		expectedEncoderFamily, strings.Join(want, ", "))

	streamsTOML := filepath.Join(runDir, "streams.toml")
	data, err := os.ReadFile(streamsTOML)
	if err != nil {
		t.Fatalf("read %s: %v", streamsTOML, err)
	}

	var vf validationFile
	if err := toml.Unmarshal(data, &vf); err != nil {
		t.Fatalf("parse %s: %v", streamsTOML, err)
	}

	all := append([]string{}, vf.Validation.H264.Working...)
	all = append(all, vf.Validation.H265.Working...)

	if len(all) == 0 {
		t.Fatalf("no working encoders in %s; validate-encoders may have failed", streamsTOML)
	}

	found := false
	for _, w := range want {
		if slices.Contains(all, w) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s family encoder in working list, none found.\nworking h264: %v\nworking h265: %v",
			expectedEncoderFamily, vf.Validation.H264.Working, vf.Validation.H265.Working)
	}
	t.Logf("working h264: %v", vf.Validation.H264.Working)
	t.Logf("working h265: %v", vf.Validation.H265.Working)
}

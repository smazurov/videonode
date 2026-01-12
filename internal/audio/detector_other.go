//go:build !linux

package audio

import "fmt"

type stubDetector struct{}

func newPlatformDetector() Detector {
	return &stubDetector{}
}

// ListDevices returns an error on unsupported platforms.
func (d *stubDetector) ListDevices() ([]Device, error) {
	return nil, fmt.Errorf("audio device enumeration not supported on this platform")
}

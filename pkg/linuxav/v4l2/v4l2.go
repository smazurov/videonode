//go:build linux

// Package v4l2 provides pure Go bindings to the Video4Linux2 (V4L2) API
// for device enumeration, format queries, and signal detection.
//
// This package does not use cgo, enabling simple cross-compilation for
// different Linux architectures (amd64, arm64, arm).
//
// # Device Enumeration
//
// Use FindDevices to discover all V4L2 video capture devices:
//
//	devices, err := v4l2.FindDevices()
//	for _, dev := range devices {
//	    fmt.Printf("%s: %s\n", dev.DevicePath, dev.DeviceName)
//	}
//
// # Format Queries
//
// Query supported formats, resolutions, and framerates:
//
//	formats, _ := v4l2.GetFormats("/dev/video0")
//	for _, fmt := range formats {
//	    resolutions, _ := v4l2.GetResolutions("/dev/video0", fmt.PixelFormat)
//	    for _, res := range resolutions {
//	        framerates, _ := v4l2.GetFramerates("/dev/video0", fmt.PixelFormat, res.Width, res.Height)
//	    }
//	}
//
// # HDMI Signal Detection
//
// For HDMI capture devices, check signal status:
//
//	status := v4l2.GetDVTimings("/dev/video0")
//	if status.State == v4l2.SignalStateLocked {
//	    fmt.Printf("Signal: %dx%d @ %.2f fps\n", status.Width, status.Height, status.FPS)
//	}
//
// # Source Change Events
//
// Wait for source change events (e.g., resolution change on HDMI):
//
//	changes, err := v4l2.WaitForSourceChange("/dev/video0", 5000) // 5 second timeout
//	if err == nil && changes > 0 {
//	    // Resolution or signal changed
//	}
package v4l2

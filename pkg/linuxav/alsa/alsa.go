//go:build linux

// Package alsa provides pure Go bindings to the ALSA (Advanced Linux Sound Architecture)
// API for audio device enumeration and capability queries.
//
// This package does not use cgo, enabling simple cross-compilation for
// different Linux architectures (amd64, arm64, arm).
//
// # Device Enumeration
//
// Use ListDevices to discover all ALSA audio capture devices:
//
//	devices, err := alsa.ListDevices()
//	for _, dev := range devices {
//	    fmt.Printf("%s: %s (%s)\n", dev.ALSADevice, dev.DeviceName, dev.CardName)
//	    fmt.Printf("  Rates: %v\n", dev.SupportedRates)
//	    fmt.Printf("  Channels: %d-%d\n", dev.MinChannels, dev.MaxChannels)
//	    fmt.Printf("  Formats: %v\n", dev.SupportedFormats)
//	}
package alsa

package streams

import (
	"fmt"
	"net"
)

const defaultRTSPHost = "127.0.0.1:8554"

// resolveRTSPHost normalizes the daemon's --streaming-rtsp-port flag
// to the host:port the GPU compose path's ffmpeg sink dials.
//
// Accepted forms:
//
//	""              → "127.0.0.1:8554" (well-known default)
//	":8654"         → "127.0.0.1:8654" (bind-on-all → dial loopback)
//	"0.0.0.0:8654"  → "127.0.0.1:8654" (any-address → dial loopback)
//	"host:port"     → unchanged
//
// Bare-port input ("8654" — no colon, no host) is rejected by falling
// back to the default rather than producing "rtsp://8654/<id>", which
// ffmpeg would (mis)interpret as host=8654 with no port and fail DNS.
func resolveRTSPHost(rtspPort string) string {
	if rtspPort == "" || rtspPort == ":" {
		return defaultRTSPHost
	}
	host, port, err := net.SplitHostPort(rtspPort)
	if err != nil {
		return defaultRTSPHost
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// getSocketPath returns the conventional ffmpeg progress socket path
// for a stream id. Used by setup.go to wire the FFmpegCollector that
// listens for `-progress` reports from supervised ffmpeg processes.
func getSocketPath(streamID string) string {
	return fmt.Sprintf("/tmp/ffmpeg-progress-%s.sock", streamID)
}

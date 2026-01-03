// Package nats provides embedded NATS messaging for inter-process communication
// between the main videonode process and stream worker processes.
//
// # Architecture
//
//   - Server: Embedded NATS server running in the main process (videonode serve)
//   - StreamClient: NATS client for stream processes (videonode stream <id>)
//   - Bridge: Subscribes to NATS subjects and publishes to the event bus
//
// # Subject Hierarchy
//
//	videonode.streams.{stream_id}.metrics   # FFmpeg metrics (client → server)
//	videonode.streams.{stream_id}.logs      # Log messages (client → server)
//	videonode.streams.{stream_id}.state     # State changes (client → server)
//	videonode.control.{stream_id}.restart   # Restart command (server → client)
//
// The package uses fire-and-forget messaging (core NATS, no JetStream).
// Stream clients gracefully degrade when NATS is unavailable.
//
// # Debugging with nats CLI
//
// Install the NATS CLI:
//
//	# macOS
//	brew install nats-io/nats-tools/nats
//
//	# Linux (download from GitHub releases)
//	curl -L https://github.com/nats-io/natscli/releases/latest/download/nats-0.1.5-linux-amd64.zip -o nats.zip
//	unzip nats.zip && sudo mv nats /usr/local/bin/
//
//	# Or via Go
//	go install github.com/nats-io/natscli/nats@latest
//
// # Useful Debug Commands
//
// Monitor all stream messages (metrics, logs, state):
//
//	nats sub "videonode.streams.>"
//
// Monitor metrics for a specific stream:
//
//	nats sub "videonode.streams.stream-001.metrics"
//
// Monitor all control commands:
//
//	nats sub "videonode.control.>"
//
// Send a restart command manually:
//
//	nats pub "videonode.control.stream-001.restart" '{"action":"restart","stream_id":"stream-001","timestamp":"2024-01-01T00:00:00Z","reason":"manual_debug"}'
//
// Check server info and connected clients:
//
//	nats server info
//	nats server list
//
// View live message statistics:
//
//	nats sub "videonode.>" --count=100
//
// Pretty-print JSON messages:
//
//	nats sub "videonode.streams.>" | jq .
//
// # Example Debug Session
//
// Terminal 1 - Start videonode with NATS:
//
//	./videonode serve
//
// Terminal 2 - Monitor all NATS traffic:
//
//	nats sub "videonode.>" -s nats://localhost:4222
//
// Terminal 3 - Manually test restart:
//
//	nats pub "videonode.control.test-stream.restart" \
//	  '{"action":"restart","stream_id":"test-stream","reason":"debug"}' \
//	  -s nats://localhost:4222
//
// # Message Formats
//
// MetricsMessage (videonode.streams.{id}.metrics):
//
//	{
//	  "stream_id": "stream-001",
//	  "timestamp": "2024-01-01T12:00:00Z",
//	  "fps": "30",
//	  "dropped_frames": "0",
//	  "duplicate_frames": "0",
//	  "processing_speed": "1.0x"
//	}
//
// LogMessage (videonode.streams.{id}.logs):
//
//	{
//	  "stream_id": "stream-001",
//	  "timestamp": "2024-01-01T12:00:00Z",
//	  "level": "info",
//	  "message": "Stream started",
//	  "source": "stdout"
//	}
//
// StateMessage (videonode.streams.{id}.state):
//
//	{
//	  "stream_id": "stream-001",
//	  "timestamp": "2024-01-01T12:00:00Z",
//	  "enabled": true,
//	  "reason": "device_ready"
//	}
//
// ControlMessage (videonode.control.{id}.restart):
//
//	{
//	  "action": "restart",
//	  "stream_id": "stream-001",
//	  "timestamp": "2024-01-01T12:00:00Z",
//	  "reason": "api_restart"
//	}
package nats

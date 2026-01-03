package nats

import (
	"encoding/json"
	"fmt"
)

// Subject prefixes for NATS topics.
const (
	SubjectStreamsPrefix = "videonode.streams"
	SubjectControlPrefix = "videonode.control"
)

// Subject returns the full NATS subject for a stream metric.
func SubjectStreamMetrics(streamID string) string {
	return fmt.Sprintf("%s.%s.metrics", SubjectStreamsPrefix, streamID)
}

// SubjectStreamLogs returns the full NATS subject for stream logs.
func SubjectStreamLogs(streamID string) string {
	return fmt.Sprintf("%s.%s.logs", SubjectStreamsPrefix, streamID)
}

// SubjectStreamState returns the full NATS subject for stream state changes.
func SubjectStreamState(streamID string) string {
	return fmt.Sprintf("%s.%s.state", SubjectStreamsPrefix, streamID)
}

// SubjectControlRestart returns the NATS subject for restart commands.
func SubjectControlRestart(streamID string) string {
	return fmt.Sprintf("%s.%s.restart", SubjectControlPrefix, streamID)
}

// MetricsMessage represents FFmpeg stream metrics sent over NATS.
type MetricsMessage struct {
	StreamID        string `json:"stream_id"`
	Timestamp       string `json:"timestamp"`
	FPS             string `json:"fps"`
	DroppedFrames   string `json:"dropped_frames"`
	DuplicateFrames string `json:"duplicate_frames"`
	ProcessingSpeed string `json:"processing_speed"`
}

// Marshal serializes the message to JSON.
func (m MetricsMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// LogMessage represents a log entry sent over NATS.
type LogMessage struct {
	StreamID  string         `json:"stream_id"`
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"` // debug, info, warn, error
	Message   string         `json:"message"`
	Source    string         `json:"source"` // stdout, stderr
	Details   map[string]any `json:"details,omitempty"`
}

// Marshal serializes the message to JSON.
func (m LogMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// StateMessage represents a stream state change sent over NATS.
type StateMessage struct {
	StreamID  string `json:"stream_id"`
	Timestamp string `json:"timestamp"`
	Enabled   bool   `json:"enabled"`
	Reason    string `json:"reason,omitempty"` // device_ready, device_offline, etc.
}

// Marshal serializes the message to JSON.
func (m StateMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// ControlMessage represents a control command sent to stream processes.
type ControlMessage struct {
	Action    string `json:"action"` // restart
	StreamID  string `json:"stream_id"`
	Timestamp string `json:"timestamp"`
	Reason    string `json:"reason,omitempty"`
}

// Marshal serializes the message to JSON.
func (m ControlMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalMetrics deserializes a MetricsMessage from JSON.
func UnmarshalMetrics(data []byte) (MetricsMessage, error) {
	var m MetricsMessage
	err := json.Unmarshal(data, &m)
	return m, err
}

// UnmarshalLog deserializes a LogMessage from JSON.
func UnmarshalLog(data []byte) (LogMessage, error) {
	var m LogMessage
	err := json.Unmarshal(data, &m)
	return m, err
}

// UnmarshalState deserializes a StateMessage from JSON.
func UnmarshalState(data []byte) (StateMessage, error) {
	var m StateMessage
	err := json.Unmarshal(data, &m)
	return m, err
}

// UnmarshalControl deserializes a ControlMessage from JSON.
func UnmarshalControl(data []byte) (ControlMessage, error) {
	var m ControlMessage
	err := json.Unmarshal(data, &m)
	return m, err
}

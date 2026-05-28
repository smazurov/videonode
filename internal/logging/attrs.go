package logging

// Canonical slog attribute keys. Use these constants instead of string
// literals to keep the attribute vocabulary consistent across the codebase.

// Entity IDs.
const (
	KeySourceID   = "source_id"
	KeyComposerID = "composer_id"
	KeyStreamID   = "stream_id"
	KeyDeviceID   = "device_id"
	KeyEntityID   = "entity_id"
	KeyPoolID     = "pool_id"
	KeyPeerID     = "peer_id"
)

// Process / pipeline.
const (
	KeyModule          = "module"
	KeyStageInstance   = "stage_instance"
	KeyPID             = "pid"
	KeyExitCode        = "exit_code"
	KeyCommand         = "command"
	KeyPipe            = "pipe"
	KeyPipelineEnabled = "pipeline_enabled"
)

// Binary paths.
const (
	KeySourceBin   = "source_bin"
	KeySinkBin     = "sink_bin"
	KeyComposerBin = "composer_bin"
	KeySourceOK    = "source_ok"
	KeySinkOK      = "sink_ok"
	KeyComposerOK  = "composer_ok"
)

// Video / capture.
const (
	KeyFourCC  = "fourcc"
	KeyWidth   = "width"
	KeyHeight  = "height"
	KeyFPS     = "fps"
	KeyCodec   = "codec"
	KeyEncoder = "encoder"
	KeyCanvas  = "canvas"
)

// Network / streaming.
const (
	KeyAddr        = "addr"
	KeyPort        = "port"
	KeyRemote      = "remote"
	KeyURL         = "url"
	KeySocket      = "socket"
	KeyUDS         = "uds"
	KeyLatencyMS   = "latency_ms"
	KeyStreamPeers = "stream_peers"
	KeyFMTP        = "fmtp"
	KeyProfileIDC  = "profile_idc"
	KeyLevelIDC    = "level_idc"
)

// Common.
const (
	KeyError          = "error"
	KeyName           = "name"
	KeyKind           = "kind"
	KeyPath           = "path"
	KeyDevice         = "device"
	KeyState          = "state"
	KeyResolution     = "resolution"
	KeyComponent      = "component"
	KeyPixelFmt       = "pixel_format"
	KeyAction         = "action"
	KeyDevPath        = "devpath"
	KeyDevType        = "devtype"
	KeyEnabled        = "enabled"
	KeyType           = "type"
	KeyServiceUser    = "service_user"
	KeyProvided       = "provided"
	KeyExpected       = "expected"
	KeyUsername       = "username"
	KeyRemoteAddr     = "remote_addr"
	KeyGroup          = "group"
	KeySignal         = "signal"
	KeyDebounce       = "debounce"
	KeyOp             = "op"
	KeyLEDType        = "led_type"
	KeyPattern        = "pattern"
	KeyInterval       = "interval"
	KeyTimeout        = "timeout"
	KeyGen            = "gen"
	KeyConsumerID     = "consumer_id"
	KeyTotalConsumers = "total_consumers"
	KeyBytesSent      = "bytes_sent"
	KeyReaderCount    = "reader_count"
	KeyInputPixFmt    = "input_pix_fmt"
	KeyStaleFor       = "stale_for"
	KeyBackoff        = "backoff"
	KeyApplyError     = "apply_error"
	KeyRollbackError  = "rollback_error"
	KeyDroppedDevices = "dropped_devices"
	KeyRef            = "ref"
	KeyFirstInput     = "first_input"
	KeyDuplicateInput = "duplicate_input"
	KeySPSLen         = "sps_len"
	KeyPPSLen         = "pps_len"
	KeyBoardModel     = "board_model"
	KeyFrom           = "from"
	KeyTo             = "to"
	KeyAttempt        = "attempt"
	KeyAttempts       = "attempts"
	KeyRetries        = "retries"
	KeyChanges        = "changes"
	KeyErrno          = "errno"
	KeyPixelclock     = "pixelclock"
	KeyInterlaced     = "interlaced"
)

// Counts / status.
const (
	KeyDeviceCount      = "device_count"
	KeyPacketsLost      = "packets_lost"
	KeySources          = "sources"
	KeyComposers        = "composers"
	KeyStreams          = "streams"
	KeyStreamsApplied   = "streams_applied"
	KeyStreamsPersisted = "streams_persisted"
	KeySuppressed       = "suppressed"
	KeyReason           = "reason"
	KeyVersion          = "version"
)

// AllowedKeys is the canonical set of attribute keys that C++ structured
// log lines may use. The Go parser warns on unknown keys at runtime.
var AllowedKeys = map[string]struct{}{
	KeySourceID: {}, KeyComposerID: {}, KeyStreamID: {},
	KeyDeviceID: {}, KeyEntityID: {}, KeyPoolID: {}, KeyPeerID: {},
	KeyModule: {}, KeyStageInstance: {}, KeyPID: {}, KeyExitCode: {},
	KeyCommand: {}, KeyPipe: {}, KeyPipelineEnabled: {},
	KeySourceBin: {}, KeySinkBin: {}, KeyComposerBin: {},
	KeySourceOK: {}, KeySinkOK: {}, KeyComposerOK: {},
	KeyFourCC: {}, KeyWidth: {}, KeyHeight: {}, KeyFPS: {},
	KeyCodec: {}, KeyEncoder: {}, KeyCanvas: {},
	KeyAddr: {}, KeyPort: {}, KeyRemote: {}, KeyURL: {},
	KeySocket: {}, KeyUDS: {}, KeyLatencyMS: {}, KeyStreamPeers: {},
	KeyFMTP: {}, KeyProfileIDC: {}, KeyLevelIDC: {},
	KeyError: {}, KeyName: {}, KeyKind: {}, KeyPath: {},
	KeyDevice: {}, KeyState: {}, KeyResolution: {},
	KeyComponent: {}, KeyPixelFmt: {},
	KeyFrom: {}, KeyTo: {},
	KeyAction: {}, KeyDevPath: {}, KeyDevType: {},
	KeyAttempt: {}, KeyAttempts: {}, KeyRetries: {}, KeyChanges: {},
	KeyDeviceCount: {}, KeyPacketsLost: {},
	KeySources: {}, KeyComposers: {}, KeyStreams: {},
	KeyStreamsApplied: {}, KeyStreamsPersisted: {},
	KeySuppressed: {}, KeyReason: {}, KeyVersion: {},
	// C++ structured log keys (not used on Go side directly).
	"fd": {}, "total": {}, "buffers": {}, "mode": {},
	"real": {}, "placeholder": {}, "glsl": {}, "gpu": {},
}

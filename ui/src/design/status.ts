// Single source of truth for semantic color mappings used across composites.
// All values reference tokens defined in tokens.dtcg.json — no raw palette classes.

export type LogLevel = "error" | "warn" | "info" | "debug";

export function logLevelClasses(level: string): string {
  switch (level) {
    case "error":
      return "text-log-error";
    case "warn":
      return "text-log-warn";
    case "debug":
      return "text-log-debug";
    case "info":
    default:
      return "text-log-info";
  }
}

export type ConnectionState = "connected" | "connecting" | "disconnected";

export function connectionStatusClasses(state: string): string {
  switch (state) {
    case "connected":
      return "bg-success";
    case "connecting":
      return "bg-warning animate-pulse";
    case "disconnected":
    default:
      return "bg-danger";
  }
}

export type Protocol = "webrtc" | "rtsp" | "srt" | "rtmp" | "composer";

// Protocol/feature badge: returns the tone name consumed by Badge's `tone` prop.
export function protocolTone(protocol: Protocol): "webrtc" | "rtsp" | "srt" | "rtmp" | "composer" {
  return protocol;
}

// Composer/encoder lifecycle pill. Mirrors the warm/cold/encoding/idle states
// the daemon reports for the producer→composer→encoder chain.
export type StageLifecycle = "warm" | "cold" | "encoding" | "idle" | "error";

export function stageLifecycleClasses(state: StageLifecycle): string {
  switch (state) {
    case "warm":
      return "bg-success";
    case "encoding":
      return "bg-accent animate-pulse";
    case "cold":
      return "bg-surface-muted text-fg-muted";
    case "error":
      return "bg-danger";
    case "idle":
    default:
      return "bg-warning";
  }
}

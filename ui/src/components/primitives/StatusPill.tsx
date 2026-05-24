import type { BadgeProps } from "../Badge";
import { Badge } from "../Badge";

// Stub for U5 — final implementation in U5 may swap tone palette.
export type StatusPillStatus =
  | "warm"
  | "cold"
  | "error"
  | "encoding"
  | "idle"
  | "unknown";

const STATUS_TONE: Record<StatusPillStatus, BadgeProps["tone"]> = {
  warm: "success",
  cold: "neutral",
  error: "danger",
  encoding: "info",
  idle: "neutral",
  unknown: "neutral",
};

const STATUS_LABEL: Record<StatusPillStatus, string> = {
  warm: "Warm",
  cold: "Cold",
  error: "Error",
  encoding: "Encoding",
  idle: "Idle",
  unknown: "Unknown",
};

interface StatusPillProps {
  status: StatusPillStatus;
  size?: BadgeProps["size"];
  label?: string;
  className?: string;
}

export function StatusPill({ status, size = "sm", label, className }: Readonly<StatusPillProps>) {
  const tone = STATUS_TONE[status];
  const badgeProps: BadgeProps = { size };
  if (tone !== undefined) badgeProps.tone = tone;
  if (className !== undefined) badgeProps.className = className;
  return <Badge {...badgeProps}>{label ?? STATUS_LABEL[status]}</Badge>;
}

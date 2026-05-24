import React from "react";
import { cva, cn, type VariantProps } from "../../utils";

const pillVariants = cva({
  base: "inline-flex items-center gap-1.5 font-medium rounded-full shrink-0 whitespace-nowrap border border-transparent",
  variants: {
    status: {
      warm: "bg-status-warm-soft text-status-warm-soft-fg",
      cold: "bg-status-cold-soft text-status-cold-soft-fg",
      error: "bg-status-error-soft text-status-error-soft-fg",
      encoding: "bg-status-encoding-soft text-status-encoding-soft-fg",
      idle: "bg-status-idle-soft text-status-idle-soft-fg",
      running: "bg-status-running-soft text-status-running-soft-fg",
      stopped: "bg-status-stopped-soft text-status-stopped-soft-fg",
    },
    size: {
      xs: "px-1.5 py-0.5 text-[10px]",
      sm: "px-2 py-0.5 text-xs",
      md: "px-2.5 py-1 text-sm",
    },
  },
  defaultVariants: { status: "idle", size: "sm" },
});

const dotVariants = cva({
  base: "inline-block rounded-full",
  variants: {
    status: {
      warm: "bg-status-warm-soft-fg",
      cold: "bg-status-cold-soft-fg",
      error: "bg-status-error-soft-fg",
      encoding: "bg-status-encoding-soft-fg animate-pulse",
      idle: "bg-status-idle-soft-fg",
      running: "bg-status-running-soft-fg animate-pulse",
      stopped: "bg-status-stopped-soft-fg",
    },
    size: {
      xs: "h-1.5 w-1.5",
      sm: "h-1.5 w-1.5",
      md: "h-2 w-2",
    },
  },
  defaultVariants: { status: "idle", size: "sm" },
});

export type StatusPillStatus = NonNullable<VariantProps<typeof pillVariants>["status"]>;

export interface StatusPillProps extends React.HTMLAttributes<HTMLSpanElement> {
  readonly status?: StatusPillStatus;
  readonly tone?: StatusPillStatus; // alias for status (back-compat)
  readonly size?: VariantProps<typeof pillVariants>["size"];
  readonly label?: React.ReactNode;
  readonly showDot?: boolean;
}

const DEFAULT_LABELS: Record<StatusPillStatus, string> = {
  warm: "Warm",
  cold: "Cold",
  error: "Error",
  encoding: "Encoding",
  idle: "Idle",
  running: "Running",
  stopped: "Stopped",
};

export function StatusPill({
  status,
  tone,
  size,
  label,
  children,
  showDot = true,
  className,
  ...props
}: Readonly<StatusPillProps>) {
  const effective: StatusPillStatus = status ?? tone ?? "idle";
  const text = children ?? label ?? DEFAULT_LABELS[effective];
  return (
    <span className={cn(pillVariants({ status: effective, size }), className)} {...props}>
      {showDot && <span aria-hidden="true" className={dotVariants({ status: effective, size })} />}
      <span>{text}</span>
    </span>
  );
}

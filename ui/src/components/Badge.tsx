import React from "react";
import { cva, cn, type VariantProps } from "../utils";

const badgeVariants = cva({
  base: "inline-flex items-center font-medium rounded shrink-0 whitespace-nowrap",
  variants: {
    tone: {
      neutral: "bg-surface-muted text-fg-muted",
      info: "bg-accent-soft text-accent-soft-fg",
      success: "bg-success/15 text-success",
      warning: "bg-warning-soft text-warning-soft-fg",
      danger: "bg-danger-soft text-danger-soft-fg",
      accent: "bg-accent text-accent-fg",
      canvas: "bg-canvas-soft text-canvas-soft-fg",
      webrtc: "bg-webrtc-soft text-webrtc-soft-fg",
      rtsp: "bg-rtsp-soft text-rtsp-soft-fg",
      srt: "bg-srt-soft text-srt-soft-fg",
      rtmp: "bg-rtmp-soft text-rtmp-soft-fg",
    },
    size: {
      xs: "px-1.5 py-0.5 text-[10px]",
      sm: "px-2 py-0.5 text-xs",
      md: "px-2.5 py-1 text-sm",
    },
  },
  defaultVariants: { tone: "neutral", size: "sm" },
});

export type BadgeProps = React.HTMLAttributes<HTMLSpanElement> &
  VariantProps<typeof badgeVariants>;

export function Badge({ tone, size, className, children, ...props }: Readonly<BadgeProps>) {
  return (
    <span className={cn(badgeVariants({ tone, size }), className)} {...props}>
      {children}
    </span>
  );
}

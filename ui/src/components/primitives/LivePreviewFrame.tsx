<<<<<<< HEAD
import React from "react";
import { ExclamationTriangleIcon } from "@heroicons/react/24/outline";
import { cn } from "../../utils";
import { Spinner } from "../Spinner";

export type LivePreviewState = "loading" | "ready" | "error" | "idle";

export interface LivePreviewFrameProps {
  readonly state?: LivePreviewState;
  readonly aspectRatio?: number;
  readonly errorMessage?: React.ReactNode;
  readonly loadingMessage?: React.ReactNode;
  readonly idleMessage?: React.ReactNode;
  readonly stats?: React.ReactNode;
  readonly statsPosition?: "top-left" | "top-right" | "bottom-left" | "bottom-right";
  readonly className?: string;
  readonly mediaClassName?: string;
  readonly children?: React.ReactNode;
}

const statsPositionClasses: Record<NonNullable<LivePreviewFrameProps["statsPosition"]>, string> = {
  "top-left": "top-2 left-2",
  "top-right": "top-2 right-2",
  "bottom-left": "bottom-2 left-2",
  "bottom-right": "bottom-2 right-2",
};

export function LivePreviewFrame({
  state = "ready",
  aspectRatio = 16 / 9,
  errorMessage,
  loadingMessage = "Loading preview…",
  idleMessage = "Preview not available",
  stats,
  statsPosition = "bottom-right",
  className,
  mediaClassName,
  children,
}: Readonly<LivePreviewFrameProps>) {
  return (
    <div
      className={cn(
        "relative w-full overflow-hidden rounded-lg bg-surface-sunken border border-border",
        className,
      )}
      style={{ aspectRatio: String(aspectRatio) }}
    >
      <div className={cn("absolute inset-0 flex items-center justify-center", mediaClassName)}>
        {children}
      </div>

      {state === "loading" && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-surface-overlay text-fg-inverse">
          <Spinner size="md" tone="current" />
          <span className="text-xs">{loadingMessage}</span>
        </div>
      )}

      {state === "error" && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-danger-soft text-danger-soft-fg p-4 text-center">
          <ExclamationTriangleIcon className="h-6 w-6" />
          <span className="text-xs">{errorMessage ?? "Preview failed"}</span>
        </div>
      )}

      {state === "idle" && (
        <div className="absolute inset-0 flex items-center justify-center text-fg-subtle text-xs">
          {idleMessage}
        </div>
      )}

      {stats && state === "ready" && (
        <div
          className={cn(
            "absolute z-10 rounded-md bg-surface-overlay text-fg-inverse text-[11px] px-2 py-1 font-mono",
            statsPositionClasses[statsPosition],
          )}
        >
          {stats}
        </div>
      )}
=======
import type { ReactNode } from 'react';
import { WebRTCPlayer } from '../webrtc';
import { cn } from '../../utils';

interface LivePreviewFrameProps {
  readonly streamId: string;
  readonly enabled?: boolean;
  readonly showStats?: boolean;
  readonly className?: string;
  readonly refreshKey?: number;
  readonly overlay?: ReactNode;
  readonly placeholder?: ReactNode;
}

export function LivePreviewFrame({
  streamId,
  enabled = true,
  showStats = false,
  className,
  refreshKey,
  overlay,
  placeholder,
}: LivePreviewFrameProps) {
  return (
    <div className={cn('relative aspect-video overflow-hidden rounded-lg bg-surface-muted', className)}>
      {enabled ? (
        <WebRTCPlayer
          key={refreshKey ?? 0}
          streamId={streamId}
          className="h-full w-full"
          showStats={showStats}
        />
      ) : (
        <div className="flex h-full w-full items-center justify-center text-sm text-fg-muted">
          {placeholder ?? 'Stream disabled'}
        </div>
      )}
      {overlay && <div className="pointer-events-none absolute inset-0">{overlay}</div>}
>>>>>>> worktree-agent-a00537f30a6b3ed35
    </div>
  );
}

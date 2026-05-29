// Domain type aliases keyed off the canonical generated API schema. Runtime
// augmentations (status, consumers, refresh keys) layer on top via the
// individual slice extensions; the base shape is whatever the server emits.

import type { components } from '../../lib/api.generated';

export type SourceData = components['schemas']['SourceData'];
export type SourceRequestData = components['schemas']['SourceData'];

// SourceStatusSnapshot mirrors pipelinectl.StatusParams, the per-source
// status payload carried on the entity `source.status` event. The entity
// envelope's payload is untyped on the wire (EntityEvent.payload is `any`),
// so the UI owns this shape rather than deriving it from the OpenAPI spec.
export interface SourceStatusSnapshot {
  device_id: string;
  ts_ms: number;
  started_at_us?: number;
  health: string;
  device: { path: string; multiplanar: boolean };
  signal: {
    has_dv_timings: boolean;
    cable_present: boolean;
    signal_locked: boolean;
    dv_timings: string;
  };
  format: {
    fourcc: string;
    w: number;
    h: number;
    fps: number;
    buffers: number;
    mode: string;
  };
  broadcast: {
    target_fps: number;
    real_frames: number;
    placeholder_frames: number;
    last_seq: number;
  };
  consumers: {
    count: number;
    live: unknown[];
    evicted: unknown[];
  };
}

export interface Source extends SourceData {
  latest_status?: SourceStatusSnapshot;
  consumers?: { kind: 'composer' | 'stream'; id: string }[];
  consumer_count?: number;
  last_status_at?: string;
  // Process start time in unix microseconds, stamped server-side from the
  // supervisor pool. Source of truth for "uptime" / "running since".
  started_at_us?: number;
  // Measured publish rate, computed in the UI from consecutive real_frames
  // counter deltas. Rounded to one decimal. undefined until two samples.
  effective_fps?: number;
}

export type Composer = components['schemas']['ComposerData'];
export type ComposerRequest = components['schemas']['ComposerData'];

export type AudioConfig = components['schemas']['AudioConfigData'];
export type EncoderConfig = components['schemas']['EncoderConfigData'];

export type Stream = components['schemas']['StreamData'];
export type StreamRequest = components['schemas']['StreamRequestData'];

// Composer convenience aliases for code that imported the literal types.
export type CanvasDims = NonNullable<Composer['canvas']>;
export type LayoutSlot = NonNullable<Composer['layout']>[number];
export type ComposerInput = NonNullable<Composer['inputs']>[number];
export type Effect = NonNullable<ComposerInput['effect']>;

// Domain type aliases keyed off the canonical generated API schema. Runtime
// augmentations (status, consumers, refresh keys) layer on top via the
// individual slice extensions; the base shape is whatever the server emits.

import type { components } from '../../lib/api.generated';

export type SourceData = components['schemas']['SourceData'];
export type SourceRequestData = components['schemas']['SourceData'];

// SourceStatusSnapshot is the typed `source.status` payload, derived from the
// OpenAPI schema (the SSE envelope is now a discriminated union, so the UI no
// longer hand-mirrors this shape).
export type SourceStatusSnapshot = components['schemas']['StatusParams'];

export interface Source extends SourceData {
  latest_status?: SourceStatusSnapshot;
  consumer_count?: number;
  last_status_at?: string;
  // Process start time in unix microseconds, stamped server-side from the
  // supervisor pool. Source of truth for "uptime" / "running since".
  started_at_us?: number;
  // Measured publish rate, computed in the UI from consecutive real_frames
  // counter deltas. Rounded to one decimal. undefined until two samples.
  effective_fps?: number;
}

export type SensorData = components['schemas']['SensorData'];
export type SensorCreateBody = components['schemas']['SensorCreateBody'];
export type SensorUpdateBody = components['schemas']['SensorUpdateBody'];

// SensorFinding is the typed `sensor.status` payload — one detection the
// daemon emitted, with what the follow/commit policy decided. The live feed of
// these is how the UI shows a sensor is actually working.
export type SensorFinding = components['schemas']['FindingEvent'];

export interface Sensor extends SensorData {
  latest_finding?: SensorFinding;
  last_finding_at?: string;
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

// Stub types mirroring the canonical Go definitions from
// plan-a-full-rewrite-linked-gray.md. These exist as a transition surface
// for U2 until U1's `pnpm gen:api` regen lands and exposes them via
// `ui/src/lib/api.generated.ts`. When the regen lands, consumers should
// re-export the generated `components["schemas"]["..."]` aliases through
// this module and delete the literal definitions below.

import type { components } from '../../lib/api.generated';

// Source — daemon-managed frame producer, often shared across composers/streams.
// Status/consumer fields are runtime augmentations populated by SSE; not
// part of the canonical Go Source type.
export interface Source {
  id: string;
  device?: string;
  test_mode?: boolean;
  created_at?: string;
  updated_at?: string;
  // Runtime fields (populated by SourceStatusEvent / consumer tracking)
  source_id?: string;
  status?: any;
  latest_status?: any;
  consumers?: { kind: 'composer' | 'stream'; id: string }[];
  consumer_count?: number;
  last_status_at?: string;
  running_since?: string;
}

export interface SourceRequest {
  id: string;
  device?: string;
  test_mode?: boolean;
}

export interface SourceList {
  sources: Source[] | null;
  count: number;
}

// Composer — N-input GLES BGRA compositor producing one canvas dma-buf.
export interface CanvasDims {
  w: number;
  h: number;
}

export interface Effect {
  type: string;
  corners?: [
    [number, number],
    [number, number],
    [number, number],
    [number, number],
  ];
}

export interface ComposerInput {
  ref: string; // "source:<id>"
  effect?: Effect;
}

export interface LayoutSlot {
  input: string; // matches ComposerInput.ref by name
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Composer {
  id: string;
  canvas: CanvasDims;
  inputs: ComposerInput[];
  layout: LayoutSlot[];
  created_at?: string;
  updated_at?: string;
}

export interface ComposerRequest {
  id: string;
  canvas: CanvasDims;
  inputs: ComposerInput[];
  layout?: LayoutSlot[];
}

export interface ComposerList {
  composers: Composer[] | null;
  count: number;
}

// Stream — encoder identity. Audio+encoder+publish per stream.
export interface AudioConfig {
  devices?: string[];
  codec?: string;
  bitrate?: string;
  filters?: string;
}

export interface EncoderConfig {
  codec?: string;
  encoder_name?: string;
  global_args?: string[];
  video_filters?: string;
  bitrate?: string;
  gop?: number;
  b_frames?: number;
  rate_control?: string;
  preset?: string;
}

export interface PublishTarget {
  type: string;
  url: string;
}

export interface Stream {
  stream_id: string;
  name?: string;
  upstream?: string; // "source:<id>" | "composer:<id>"
  audio?: AudioConfig;
  encoder?: EncoderConfig;
  publish?: PublishTarget[];
  custom_encoder_args?: string;
  enabled?: boolean;
  rtsp_url?: string;
  srt_url?: string;
  created_at?: string;
  updated_at?: string;
}

export interface StreamRequest {
  stream_id: string;
  name?: string;
  upstream?: string;
  audio?: AudioConfig;
  encoder?: EncoderConfig;
  publish?: PublishTarget[];
  custom_encoder_args?: string;
}

export interface StreamList {
  streams: Stream[] | null;
  count: number;
}

// Legacy transition bag — fields that existing `StreamData` exposes today
// but the canonical slim Stream drops. Stored alongside the canonical
// Stream so consumers waiting on U12/U13/U14 rewrites keep compiling. The
// integrator removes this intersection when the dependent UI units land.
export type LegacyStreamFields = Partial<
  Omit<components['schemas']['StreamData'], 'stream_id' | 'name' | 'enabled' | 'rtsp_url' | 'srt_url'>
>;

export type StoredStream = Stream & LegacyStreamFields;

// Domain type aliases keyed off the canonical generated API schema. Runtime
// augmentations (status, consumers, refresh keys) layer on top via the
// individual slice extensions; the base shape is whatever the server emits.

import type { components } from '../../lib/api.generated';

export type SourceData = components['schemas']['SourceData'];
export type SourceRequestData = components['schemas']['SourceData'];
export type SourceStatusSnapshot = components['schemas']['StatusParams'];

import type { StatusPillStatus } from '../../components/primitives/StatusPill';

export interface Source extends SourceData {
  // Runtime fields populated by SourceStatusEvent / consumer tracking.
  status?: StatusPillStatus;
  latest_status?: SourceStatusSnapshot;
  consumers?: { kind: 'composer' | 'stream'; id: string }[];
  consumer_count?: number;
  last_status_at?: string;
  running_since?: string;
}

export type Composer = components['schemas']['ComposerPayload'];
export type ComposerRequest = components['schemas']['ComposerPayload'];

export type AudioConfig = components['schemas']['AudioConfigData'];
export type EncoderConfig = components['schemas']['EncoderConfigData'];
export type PublishTarget = components['schemas']['PublishTargetData'];

export type Stream = components['schemas']['StreamData'];
export type StreamRequest = components['schemas']['StreamRequestData'];

// Composer convenience aliases for code that imported the literal types.
export type CanvasDims = NonNullable<Composer['canvas']>;
export type LayoutSlot = NonNullable<Composer['layout']>[number];
export type ComposerInput = NonNullable<Composer['inputs']>[number];
export type Effect = NonNullable<ComposerInput['effect']>;

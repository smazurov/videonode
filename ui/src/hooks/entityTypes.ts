// Pure types + tiny helpers shared by every entity store. Lives in
// its own file to avoid circular imports (a slice that handles
// EntityEvents needs these symbols; the dispatcher in entityDispatch.ts
// needs the same symbols PLUS the stores themselves).

import type { components, operations } from '../lib/api.generated';

type EntitySSEData =
  operations['events-stream']['responses'][200]['content']['text/event-stream'][number];

// EntityEvent is the discriminated union carried on the single "entity" SSE
// event, narrowing on the `type` tag ("<entity>.<action>"). Derived from the
// generated schema — the backend owns the shape end-to-end.
export type EntityEvent = Extract<EntitySSEData, { event: 'entity' }>['data'];

export type SourceEvent = Extract<EntityEvent, { type: `source.${string}` }>;
export type ComposerEvent = Extract<EntityEvent, { type: `composer.${string}` }>;
export type StreamEvent = Extract<EntityEvent, { type: `stream.${string}` }>;
export type RecordingEvent = Extract<EntityEvent, { type: `recording.${string}` }>;

// Payload aliases for the typed slots stores keep keyed by id.
export type StatusParams = components['schemas']['StatusParams'];
export type SourceConsumers = components['schemas']['SourceConsumersInfo'];
export type StreamStatus = components['schemas']['StreamStatusPayload'];
export type StreamMetrics = components['schemas']['StreamMetricsPayload'];
export type StreamConsumers = components['schemas']['StreamConsumersPayload'];

// assertNever turns a missing union case into a compile error.
export function assertNever(x: never): never {
  throw new Error(`unhandled entity event: ${String(x)}`);
}

// Pure types + tiny helpers shared by every entity store. Lives in
// its own file to avoid circular imports (a slice that handles
// EntityEvents needs these symbols; the dispatcher in entityDispatch.ts
// needs the same symbols PLUS the stores themselves).

import type { components } from '../lib/api.generated';

export type EntityEvent = components['schemas']['EntityEvent'];

// Canonical entity type names. Must match the strings registered in
// `internal/api/server.go` via events.Register(...). Updating this
// union is the only UI-side step required when a new entity is added
// backend-side; the satisfies constraint in entityDispatch.ts forces
// the dispatcher table to be updated in lockstep.
export type EntityType = 'source' | 'composer' | 'stream';

// Canonical action names. Must match the constants in
// `internal/events/registry.go` (ActionCreated/Updated/Deleted/
// Status/Metrics/Consumers). The exhaustive switch in each store's
// applyEntityEvent turns a missing case into a compile error.
export type EntityAction =
  | 'created'
  | 'updated'
  | 'deleted'
  | 'status'
  | 'metrics'
  | 'consumers';

// Exhaustive action check. Use inside each store's applyEntityEvent
// implementation: `default: assertNever(action)`. Adding a new
// EntityAction without handling it produces a TS compile error.
export function assertNever(x: never): never {
  throw new Error(`unhandled entity action: ${String(x)}`);
}

// Contract every entity store must implement. The dispatcher only
// needs this much; each store still exports its own typed selectors
// for components to consume.
export interface EntityStoreLike {
  applyEntityEvent: (
    action: EntityAction,
    id: string,
    payload: unknown,
  ) => void;
}

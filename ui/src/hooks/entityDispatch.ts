// Generic dispatcher for the uniform EntityEvent envelope emitted by
// the backend (`internal/events/registry.go`). Every per-entity SSE
// event flows through this one code path; adding a new entity means
// adding ONE entry to ENTITY_STORES below.
//
// Static-check story: ENTITY_STORES is constrained by `satisfies
// Record<EntityType, EntityStoreLike>`. Adding "newentity" to
// EntityType (entityTypes.ts) without adding it here = compile error
// naming the missing key.

import { useSourceStore } from './useSourceStore';
import { useComposerStore } from './useComposerStore';
import { useStreamStore } from './useStreamStore';
import type {
  EntityAction,
  EntityEvent,
  EntityStoreLike,
  EntityType,
} from './entityTypes';

const ENTITY_STORES = {
  source: {
    applyEntityEvent: (action, id, payload) =>
      useSourceStore.getState().applyEntityEvent(action, id, payload),
  },
  composer: {
    applyEntityEvent: (action, id, payload) =>
      useComposerStore.getState().applyEntityEvent(action, id, payload),
  },
  stream: {
    applyEntityEvent: (action, id, payload) =>
      useStreamStore.getState().applyEntityEvent(action, id, payload),
  },
} as const satisfies Record<EntityType, EntityStoreLike>;

export function dispatchEntityEvent(event: EntityEvent): void {
  const entityType = event.entity_type as EntityType;
  const action = event.action as EntityAction;
  const store = ENTITY_STORES[entityType];
  if (!store) {
    // Unknown entity_type — backend emitted an entity the UI doesn't
    // know about yet. Log and drop; after regen + entityTypes.ts
    // update, this case should never fire in production.
    if (import.meta.env.DEV) {
      console.warn(
        `[entityDispatch] unknown entity_type=${entityType} action=${action}; update ui/src/hooks/entityTypes.ts EntityType + ui/src/hooks/entityDispatch.ts ENTITY_STORES`,
      );
    }
    return;
  }
  store.applyEntityEvent(action, event.id, event.payload);
}

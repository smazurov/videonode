// Generic dispatcher for the single "entity" SSE event. The wire payload is a
// discriminated union (entityTypes.ts EntityEvent); routing narrows on the
// `type` prefix into the owning store, where the per-type switch narrows the
// payload. Adding a new entity is a compile error here until handled (the
// final `assertNever`-style check below).

import { useSourceStore } from './useSourceStore';
import { useComposerStore } from './useComposerStore';
import { useStreamStore } from './useStreamStore';
import { useSensorStore } from './useSensorStore';
import type {
  ComposerEvent,
  EntityEvent,
  SensorEvent,
  SourceEvent,
  StreamEvent,
} from './entityTypes';

function isSourceEvent(e: EntityEvent): e is SourceEvent {
  return e.type.startsWith('source.');
}
function isComposerEvent(e: EntityEvent): e is ComposerEvent {
  return e.type.startsWith('composer.');
}
function isStreamEvent(e: EntityEvent): e is StreamEvent {
  return e.type.startsWith('stream.');
}
function isSensorEvent(e: EntityEvent): e is SensorEvent {
  return e.type.startsWith('sensor.');
}

export function dispatchEntityEvent(event: EntityEvent): void {
  if (isSourceEvent(event)) {
    useSourceStore.getState().applyEntityEvent(event);
  } else if (isComposerEvent(event)) {
    useComposerStore.getState().applyEntityEvent(event);
  } else if (isStreamEvent(event)) {
    useStreamStore.getState().applyEntityEvent(event);
  } else if (isSensorEvent(event)) {
    useSensorStore.getState().applyEntityEvent(event);
  } else {
    // Compile-time exhaustiveness: a new entity prefix makes `event` non-never
    // here and fails the build until it's routed above.
    const unhandled: never = event;
    if (import.meta.env.DEV) {
      console.warn('[entityDispatch] unhandled entity event', unhandled);
    }
  }
}

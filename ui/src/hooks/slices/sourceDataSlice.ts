import { StateCreator } from 'zustand';
import type { components } from '../../lib/api.generated';

type SourceStatusEvent = components["schemas"]["SourceStatusEvent"];
type StatusParams = components["schemas"]["StatusParams"];

export type SourceStatus = 'warm' | 'cold' | 'error' | 'unknown';

export interface SourceConsumerRef {
  kind: 'composer' | 'stream';
  id: string;
}

export interface SourceEntry {
  source_id: string;
  device?: string | undefined;
  test_mode: boolean;
  status: SourceStatus;
  last_status_at?: string | undefined;
  running_since?: string | undefined;
  consumer_count: number;
  consumers: SourceConsumerRef[];
  latest_status?: StatusParams | undefined;
}

export interface SourceDataSlice {
  sourceIds: string[];
  sourcesById: Record<string, SourceEntry>;
  applyStatusEvent: (event: SourceStatusEvent) => void;
  setConsumers: (sourceId: string, consumers: SourceConsumerRef[]) => void;
  upsertSource: (entry: Partial<SourceEntry> & { source_id: string }) => void;
  removeSource: (sourceId: string) => void;
  getSourceById: (id: string) => SourceEntry | undefined;
}

function sortIds(ids: string[]): string[] {
  return [...ids].sort((a, b) => a.localeCompare(b));
}

function statusFromHealth(health: string | undefined): SourceStatus {
  switch (health) {
    case 'ok':
    case 'healthy':
    case 'streaming':
      return 'warm';
    case 'error':
    case 'fault':
      return 'error';
    case 'idle':
    case 'cold':
    case 'stopped':
      return 'cold';
    default:
      return 'unknown';
  }
}

export const createSourceDataSlice: StateCreator<
  SourceDataSlice,
  [],
  [],
  SourceDataSlice
> = (set, get) => ({
  sourceIds: [],
  sourcesById: {},

  applyStatusEvent: (event) => {
    const id = event.device_id;
    if (!id) return;
    const status = event.status;
    set((state) => {
      const existed = !!state.sourcesById[id];
      const prev = state.sourcesById[id];
      const next: SourceEntry = {
        source_id: id,
        device: status?.device?.path ?? prev?.device,
        test_mode: prev?.test_mode ?? false,
        status: statusFromHealth(status?.health),
        last_status_at: event.timestamp,
        running_since: prev?.running_since ?? event.timestamp,
        consumer_count: status?.consumers?.count ?? prev?.consumer_count ?? 0,
        consumers: prev?.consumers ?? [],
        latest_status: status,
      };
      const sourcesById = { ...state.sourcesById, [id]: next };
      const sourceIds = existed
        ? sortIds(state.sourceIds)
        : sortIds([...state.sourceIds, id]);
      return { sourceIds, sourcesById };
    });
  },

  setConsumers: (sourceId, consumers) => {
    set((state) => {
      const prev = state.sourcesById[sourceId];
      if (!prev) return state;
      return {
        sourcesById: {
          ...state.sourcesById,
          [sourceId]: { ...prev, consumers, consumer_count: consumers.length },
        },
      };
    });
  },

  upsertSource: (entry) => {
    set((state) => {
      const existed = !!state.sourcesById[entry.source_id];
      const prev = state.sourcesById[entry.source_id];
      const merged: SourceEntry = {
        source_id: entry.source_id,
        device: entry.device ?? prev?.device,
        test_mode: entry.test_mode ?? prev?.test_mode ?? false,
        status: entry.status ?? prev?.status ?? 'unknown',
        last_status_at: entry.last_status_at ?? prev?.last_status_at,
        running_since: entry.running_since ?? prev?.running_since,
        consumer_count: entry.consumer_count ?? prev?.consumer_count ?? 0,
        consumers: entry.consumers ?? prev?.consumers ?? [],
        latest_status: entry.latest_status ?? prev?.latest_status,
      };
      const sourcesById = { ...state.sourcesById, [entry.source_id]: merged };
      const sourceIds = existed
        ? sortIds(state.sourceIds)
        : sortIds([...state.sourceIds, entry.source_id]);
      return { sourceIds, sourcesById };
    });
  },

  removeSource: (sourceId) => {
    set((state) => {
      if (!state.sourcesById[sourceId]) return state;
      const sourcesById = { ...state.sourcesById };
      delete sourcesById[sourceId];
      return {
        sourcesById,
        sourceIds: state.sourceIds.filter((id) => id !== sourceId),
      };
    });
  },

  getSourceById: (id) => get().sourcesById[id],
});

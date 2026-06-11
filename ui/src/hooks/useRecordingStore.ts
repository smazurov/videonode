import { create } from 'zustand';
import { api } from '../lib/api';
import type { components } from '../lib/api.generated';
import { assertNever, type RecordingEvent } from './entityTypes';

type Recording = components['schemas']['RecordingStatusData'];

const keyOf = (r: Recording): string => `${r.stream_id}/${r.recording_id}`;

interface RecordingStore {
  recordingsById: Record<string, Recording>;
  loaded: boolean;
  fetchRecordings: () => Promise<void>;
  upsertRecording: (r: Recording) => void;
  removeRecording: (id: string) => void;
  applyEntityEvent: (event: RecordingEvent) => void;
}

// useRecordingStore mirrors recording sessions, seeded by fetchRecordings and
// kept live by recording.* entity events (created/updated on start/progress/
// stop, deleted on file removal).
export const useRecordingStore = create<RecordingStore>()((set, get) => ({
  recordingsById: {},
  loaded: false,

  fetchRecordings: async () => {
    const { data } = await api.GET('/api/recordings');
    if (!data) return;
    const byId: Record<string, Recording> = {};
    for (const r of data.recordings ?? []) byId[keyOf(r)] = r;
    set({ recordingsById: byId, loaded: true });
  },

  upsertRecording: (r) =>
    set((s) => ({ recordingsById: { ...s.recordingsById, [keyOf(r)]: r } })),

  removeRecording: (id) =>
    set((s) => {
      if (!(id in s.recordingsById)) return s;
      const next = { ...s.recordingsById };
      delete next[id];
      return { recordingsById: next };
    }),

  applyEntityEvent: (event) => {
    switch (event.type) {
      case 'recording.created':
      case 'recording.updated':
        get().upsertRecording(event.payload);
        return;
      case 'recording.deleted':
        get().removeRecording(event.id);
        return;
      default:
        assertNever(event);
    }
  },
}));

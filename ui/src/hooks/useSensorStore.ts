// Sensor store: first-class perception entities. Mirrors the source store's
// three concerns (data, ui, api) in one file. Beyond CRUD, it keeps a live ring
// of recent findings per sensor (fed by the `sensor.status` SSE arm) so the UI
// can show, on every frame, that a sensor is actually working — the whole point
// of making sensors observable.
import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

import { apiSensors, unwrap } from '../lib/api';
import type {
  Sensor,
  SensorCreateBody,
  SensorFinding,
  SensorUpdateBody,
} from './slices/types';
import type { SensorEvent } from './entityTypes';

const SENSORS = '/api/sensors';
const SENSOR = '/api/sensors/{sensor_id}';

// Per-sensor cap on the retained live findings feed.
const MAX_FINDINGS = 50;

export interface SensorStore {
  sensorIds: string[];
  sensorsById: Record<string, Sensor>;
  recentFindingsById: Record<string, SensorFinding[]>;

  loading: boolean;
  error: string | null;
  lastUpdated: Date | null;

  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  setSensors: (sensors: Sensor[] | null) => void;
  addSensor: (sensor: Sensor) => void;
  removeSensor: (sensorId: string) => void;
  applyEntityEvent: (event: SensorEvent) => void;

  fetchSensors: () => Promise<void>;
  getSensor: (sensorId: string) => Promise<Sensor>;
  createSensor: (request: SensorCreateBody) => Promise<Sensor>;
  updateSensor: (sensorId: string, data: SensorUpdateBody) => Promise<Sensor>;
  deleteSensor: (sensorId: string) => Promise<void>;
}

const sortedIds = (byId: Record<string, Sensor>): string[] =>
  Object.keys(byId).sort((a, b) => a.localeCompare(b));

export const useSensorStore = create<SensorStore>()(
  subscribeWithSelector((set, get) => ({
    sensorIds: [],
    sensorsById: {},
    recentFindingsById: {},
    loading: false,
    error: null,
    lastUpdated: null,

    setLoading: (loading) => set({ loading }),
    setError: (error) => set({ error }),

    setSensors: (sensors) => {
      const byId: Record<string, Sensor> = {};
      for (const s of sensors ?? []) {
        if (s.id) byId[s.id] = s;
      }
      set({ sensorsById: byId, sensorIds: sortedIds(byId), lastUpdated: new Date() });
    },

    addSensor: (sensor) => {
      if (!sensor.id) return;
      set((state) => {
        const prev = state.sensorsById[sensor.id];
        const byId = {
          ...state.sensorsById,
          // Preserve live-only fields (latest finding) across a CRUD refresh.
          [sensor.id]: { ...prev, ...sensor },
        };
        return { sensorsById: byId, sensorIds: sortedIds(byId), lastUpdated: new Date() };
      });
    },

    removeSensor: (sensorId) => {
      set((state) => {
        if (!(sensorId in state.sensorsById)) return state;
        const byId = { ...state.sensorsById };
        delete byId[sensorId];
        const findings = { ...state.recentFindingsById };
        delete findings[sensorId];
        return {
          sensorsById: byId,
          sensorIds: sortedIds(byId),
          recentFindingsById: findings,
          lastUpdated: new Date(),
        };
      });
    },

    applyEntityEvent: (event) => {
      switch (event.type) {
        case 'sensor.created':
        case 'sensor.updated':
          get().addSensor(event.payload);
          break;
        case 'sensor.deleted':
          get().removeSensor(event.id);
          break;
        case 'sensor.status': {
          const finding = event.payload;
          const id = event.id;
          set((state) => {
            const prevList = state.recentFindingsById[id] ?? [];
            const nextList = [finding, ...prevList].slice(0, MAX_FINDINGS);
            const prevSensor = state.sensorsById[id];
            const sensorsById = prevSensor
              ? {
                  ...state.sensorsById,
                  [id]: {
                    ...prevSensor,
                    latest_finding: finding,
                    last_finding_at: event.timestamp,
                  },
                }
              : state.sensorsById;
            return {
              recentFindingsById: { ...state.recentFindingsById, [id]: nextList },
              sensorsById,
            };
          });
          break;
        }
        default: {
          const unhandled: never = event;
          if (import.meta.env.DEV) {
            console.warn('[useSensorStore] unhandled sensor event', unhandled);
          }
        }
      }
    },

    fetchSensors: async () => {
      const { setLoading, setError, setSensors, sensorIds } = get();
      const hasExisting = sensorIds.length > 0;
      if (!hasExisting) setLoading(true);
      try {
        const data = unwrap(await apiSensors.GET(SENSORS), 'Failed to fetch sensors');
        setSensors(data.sensors ?? null);
        setError(null);
      } catch (error) {
        setError(error instanceof Error ? error.message : 'Failed to fetch sensors');
      } finally {
        if (!hasExisting) setLoading(false);
      }
    },

    getSensor: async (sensorId) =>
      unwrap(
        await apiSensors.GET(SENSOR, { params: { path: { sensor_id: sensorId } } }),
        'Failed to get sensor',
      ),

    createSensor: async (request) => {
      const data = unwrap(
        await apiSensors.POST(SENSORS, { body: request }),
        'Failed to create sensor',
      );
      get().addSensor(data);
      return data;
    },

    updateSensor: async (sensorId, data) => {
      const sensor = unwrap(
        await apiSensors.PATCH(SENSOR, {
          params: { path: { sensor_id: sensorId } },
          body: data,
        }),
        'Failed to update sensor',
      );
      get().addSensor(sensor);
      return sensor;
    },

    deleteSensor: async (sensorId) => {
      unwrap(
        await apiSensors.DELETE(SENSOR, { params: { path: { sensor_id: sensorId } } }),
        'Failed to delete sensor',
      );
      get().removeSensor(sensorId);
    },
  })),
);

export type { Sensor, SensorFinding };

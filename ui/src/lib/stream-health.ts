import type { components } from './api.generated';
import type { StatusPillStatus } from '../components/primitives/StatusPill';

export type StreamHealth = components['schemas']['StreamHealth'];
export type ActivityHealth = components['schemas']['ActivityHealth'];

export function latestHealth(
  rest: StreamHealth | undefined,
  pushed: StreamHealth | undefined,
): StreamHealth | undefined {
  if (!rest) return pushed;
  if (!pushed) return rest;
  return Date.parse(rest.sampled_at) > Date.parse(pushed.sampled_at) ? rest : pushed;
}

export function encoderActive(health: StreamHealth): boolean {
  return health.encoder.state === 'running' || health.encoder.state === 'starting';
}

export function ffmpegStatsFresh(health: StreamHealth): boolean {
  return encoderActive(health)
    && health.encoder.ffmpeg.progress.state === 'flowing'
    && health.encoder.ffmpeg.frames.state === 'flowing';
}

export function activityLabel(activity: ActivityHealth): {
  label: string;
  tone: StatusPillStatus;
  detail: string;
} {
  const age = activity.age_ms == null ? '' : `${(activity.age_ms / 1000).toFixed(1)}s ago`;
  switch (activity.state) {
    case 'flowing':
      return { label: 'Flowing', tone: 'running', detail: `Last arrival ${age}` };
    case 'stalled':
      return { label: 'Stalled', tone: 'error', detail: `Last arrival ${age}` };
    case 'never_received':
      return { label: 'Waiting', tone: 'starting', detail: 'Nothing received this run' };
    case 'unsupported':
      return { label: 'Packet-only', tone: 'idle', detail: 'Access-unit tracking unavailable for this codec' };
  }
}

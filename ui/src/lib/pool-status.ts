import type { StatusPillStatus } from '../components/primitives/StatusPill';
import type { components } from './api.generated';

export type ProcessStatus = NonNullable<components['schemas']['SourceData']['status']>;

export function poolStateToPill(state: string | undefined): StatusPillStatus {
  return (state as StatusPillStatus) ?? 'idle';
}

// isRestartable reports whether a supervised process is live enough to bounce
// via POST /api/processes/{id}/restart. Only running, mid-start, or crashed
// processes qualify; idle (lazy/off) and stopping are skipped so the pool-keyed
// restart endpoint never 404s on a process that isn't there.
export function isRestartable(status: string | undefined): boolean {
  return status === 'running' || status === 'starting' || status === 'error';
}

// LIVENESS_PILL maps a source-reported liveness token (decoupled from the
// process pool state) to a pill appearance + label. Unknown tokens fall
// back to an idle pill carrying the raw token as its label.
const LIVENESS_PILL: Record<string, { status: StatusPillStatus; label: string }> = {
  live: { status: 'running', label: 'Live' },
  transitioning: { status: 'warm', label: 'Transitioning' },
  initializing: { status: 'warm', label: 'Initializing' },
  no_cable: { status: 'warm', label: 'No cable' },
  no_signal: { status: 'warm', label: 'No signal' },
  offline: { status: 'stopped', label: 'Offline' },
  unknown: { status: 'idle', label: 'Unknown' },
};

export function livenessToPill(token: string | undefined): {
  status: StatusPillStatus;
  label: string;
} {
  return LIVENESS_PILL[token ?? ''] ?? { status: 'idle', label: token ?? 'Unknown' };
}

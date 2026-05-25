import type { StatusPillStatus } from '../components/primitives/StatusPill';
import type { components } from './api.generated';

export type ProcessStatus = NonNullable<components['schemas']['SourceData']['status']>;

export function poolStateToPill(state: string | undefined): StatusPillStatus {
  return (state as StatusPillStatus) ?? 'idle';
}

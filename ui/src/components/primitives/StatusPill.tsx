import type { ReactNode } from 'react';
import { cn } from '../../utils';

export type StatusPillTone = 'running' | 'idle' | 'warm' | 'cold' | 'error' | 'neutral';

const MUTED_NEUTRAL = 'bg-surface-muted text-fg-muted';
const COLD_BG = 'bg-fg-subtle';

const toneClasses: Record<StatusPillTone, string> = {
  running: 'bg-success/15 text-success',
  idle: MUTED_NEUTRAL,
  warm: 'bg-accent-soft text-accent-soft-fg',
  cold: 'bg-surface-muted text-fg-subtle',
  error: 'bg-danger-soft text-danger-soft-fg',
  neutral: MUTED_NEUTRAL,
};

const dotClasses: Record<StatusPillTone, string> = {
  running: 'bg-success',
  idle: COLD_BG,
  warm: 'bg-accent',
  cold: COLD_BG,
  error: 'bg-danger',
  neutral: COLD_BG,
};

interface StatusPillProps {
  readonly tone: StatusPillTone;
  readonly children: ReactNode;
  readonly className?: string;
}

export function StatusPill({ tone, children, className }: StatusPillProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium',
        toneClasses[tone],
        className,
      )}
    >
      <span className={cn('h-1.5 w-1.5 rounded-full', dotClasses[tone])} aria-hidden="true" />
      {children}
    </span>
  );
}

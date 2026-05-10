import type { StatsSample, QualityScore } from './types';

const BAR_WIDTH = 16;

function getQualityColor(quality: QualityScore): string {
  switch (quality) {
    case 'excellent': return 'bg-success';
    case 'good': return 'bg-success/80';
    case 'fair': return 'bg-warning';
    case 'poor': return 'bg-danger';
    default: return 'bg-fg-subtle';
  }
}

function getFrameColor(frames: number): string {
  if (frames >= 25) return 'bg-success';
  if (frames >= 15) return 'bg-warning';
  if (frames > 0) return 'bg-danger';
  return 'bg-danger-hover';
}

interface HistoryBarProps {
  readonly samples: StatsSample[];
  readonly getValue: (s: StatsSample) => number;
  readonly maxValue: number;
  readonly getColor?: (s: StatsSample) => string;
  readonly label?: string;
  readonly inline?: boolean;
}

export function HistoryBar({
  samples,
  getValue,
  maxValue,
  getColor = (s) => getQualityColor(s.quality),
  label,
  inline,
}: HistoryBarProps) {
  const recentSamples = samples.slice(-BAR_WIDTH);
  const emptySlots = Math.max(0, BAR_WIDTH - recentSamples.length);

  const bars = (
    <div className={`flex h-3 ${inline ? 'mr-2 inline-flex' : ''}`}>
      {Array.from({ length: emptySlots }).map((_, i) => (
        <div key={`empty-${i}`} className="w-2 h-full bg-surface-muted" />
      ))}
      {recentSamples.map((sample, i) => {
        const ratio = Math.min(getValue(sample) / maxValue, 1);
        return (
          <div key={`sample-${i}`} className="w-2 h-full bg-surface-muted overflow-hidden flex flex-col-reverse">
            <div className={`w-full ${getColor(sample)}`} style={{ height: `${ratio * 100}%` }} />
          </div>
        );
      })}
    </div>
  );

  if (!label) return bars;

  return (
    <div className="flex items-center gap-2">
      {bars}
      <span className="text-fg-muted">{label}</span>
    </div>
  );
}

export function FramesHistoryBar({ samples }: { readonly samples: StatsSample[] }) {
  return (
    <HistoryBar
      samples={samples}
      getValue={(s) => s.framesDecodedDelta}
      maxValue={30}
      getColor={(s) => getFrameColor(s.framesDecodedDelta)}
      inline
    />
  );
}

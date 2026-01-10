import type { StatsSample, QualityScore } from './types';

const BAR_WIDTH = 16;

function getQualityColor(quality: QualityScore): string {
  switch (quality) {
    case 'excellent': return 'bg-green-500';
    case 'good': return 'bg-green-400';
    case 'fair': return 'bg-yellow-500';
    case 'poor': return 'bg-red-500';
    default: return 'bg-gray-600';
  }
}

function getFrameColor(frames: number): string {
  if (frames >= 25) return 'bg-green-500';
  if (frames >= 15) return 'bg-yellow-500';
  if (frames > 0) return 'bg-red-500';
  return 'bg-red-700';
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
        <div key={`empty-${i}`} className="w-2 h-full bg-gray-700" />
      ))}
      {recentSamples.map((sample, i) => {
        const ratio = Math.min(getValue(sample) / maxValue, 1);
        return (
          <div key={`sample-${i}`} className="w-2 h-full bg-gray-700 overflow-hidden flex flex-col-reverse">
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
      <span className="text-gray-300">{label}</span>
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

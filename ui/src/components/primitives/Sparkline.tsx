import { useMemo } from 'react';
import { Area, AreaChart, XAxis, YAxis } from 'recharts';
import { cn } from '../../utils';

interface SparklineProps {
  readonly values: ReadonlyArray<number | null>;
  readonly width: number;
  readonly height: number;
  /** Positioning plus a text color class; the chart draws in currentColor. */
  readonly className?: string;
  readonly topMargin?: number;
}

// Step-area trend, one step per sample, scaled from zero with 15% headroom so
// a steady rate reads flat and a drop is visibly a drop. Null samples leave gaps.
export function Sparkline({ values, width, height, className, topMargin = 4 }: SparklineProps) {
  const data = useMemo(() => values.map((y, x) => ({ x, y })), [values]);
  const maxY = useMemo(() => {
    let m = 0;
    for (const v of values) if (v != null && v > m) m = v;
    return m > 0 ? m * 1.15 : 1;
  }, [values]);

  if (data.length === 0) return null;
  return (
    <div className={cn('pointer-events-none', className)} aria-hidden="true">
      <AreaChart width={width} height={height} data={data} margin={{ top: topMargin, right: 0, bottom: 0, left: 0 }}>
        <XAxis type="number" dataKey="x" domain={[0, Math.max(data.length - 1, 1)]} hide />
        <YAxis type="number" domain={[0, maxY]} hide />
        <Area
          type="step"
          dataKey="y"
          stroke="currentColor"
          strokeWidth={1}
          fill="currentColor"
          fillOpacity={0.16}
          isAnimationActive={false}
          connectNulls={false}
          dot={false}
          activeDot={false}
        />
      </AreaChart>
    </div>
  );
}

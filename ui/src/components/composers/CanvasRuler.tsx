interface CanvasRulerProps {
  /** Canvas dimension in canvas px (W for top ruler, H for left). */
  size: number;
  /** Display length of the ruler in screen px. */
  displaySize: number;
  orientation: 'horizontal' | 'vertical';
  /** Tick interval in canvas px (e.g. 100). */
  step?: number;
}

// Lightweight pixel ruler shown along the top/left of the canvas editor.
// Pure SVG — sub-divisions in canvas-px space mapped to screen-px via scale.
export function CanvasRuler({
  size,
  displaySize,
  orientation,
  step = 100,
}: Readonly<CanvasRulerProps>) {
  const scale = displaySize / Math.max(1, size);
  const ticks: number[] = [];
  const remainder = size % step;
  const skipLast = remainder > 0 && remainder < step * 0.6;
  for (let v = 0; v <= size; v += step) {
    if (skipLast && v + step > size && v !== 0) continue;
    ticks.push(v);
  }
  if (remainder !== 0) ticks.push(size);

  if (orientation === 'horizontal') {
    return (
      <svg
        width={displaySize}
        height={16}
        className="block text-fg-subtle"
        aria-hidden="true"
      >
        <rect width={displaySize} height={16} fill="transparent" />
        {ticks.map((v) => {
          const x = v * scale;
          const nearEnd = x > displaySize - 30;
          return (
            <g key={v}>
              <line x1={x} y1={10} x2={x} y2={16} stroke="currentColor" strokeWidth={1} />
              <text
                x={nearEnd ? x - 2 : x + 2}
                y={9}
                fontSize={9}
                fill="currentColor"
                fontFamily="monospace"
                textAnchor={nearEnd ? 'end' : 'start'}
              >
                {v}
              </text>
            </g>
          );
        })}
      </svg>
    );
  }

  return (
    <svg
      width={28}
      height={displaySize}
      className="block text-fg-subtle"
      aria-hidden="true"
    >
      {ticks.map((v) => {
        const y = v * scale;
        const nearEnd = y > displaySize - 12;
        return (
          <g key={v}>
            <line x1={22} y1={y} x2={28} y2={y} stroke="currentColor" strokeWidth={1} />
            <text x={20} y={nearEnd ? y - 2 : y + 9} fontSize={9} fill="currentColor" fontFamily="monospace" textAnchor="end">
              {v}
            </text>
          </g>
        );
      })}
    </svg>
  );
}

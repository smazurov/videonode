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
  for (let v = 0; v <= size; v += step) ticks.push(v);

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
          return (
            <g key={v}>
              <line x1={x} y1={10} x2={x} y2={16} stroke="currentColor" strokeWidth={1} />
              <text x={x + 2} y={9} fontSize={9} fill="currentColor" fontFamily="monospace">
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
      width={20}
      height={displaySize}
      className="block text-fg-subtle"
      aria-hidden="true"
    >
      <rect width={20} height={displaySize} fill="transparent" />
      {ticks.map((v) => {
        const y = v * scale;
        return (
          <g key={v}>
            <line x1={14} y1={y} x2={20} y2={y} stroke="currentColor" strokeWidth={1} />
            <text x={2} y={y + 8} fontSize={9} fill="currentColor" fontFamily="monospace">
              {v}
            </text>
          </g>
        );
      })}
    </svg>
  );
}

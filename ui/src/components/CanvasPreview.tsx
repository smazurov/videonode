import type { CanvasDims, ComposerInput, LayoutSlot } from '../lib/composer-types';

interface CanvasPreviewProps {
  canvas: CanvasDims;
  inputs: readonly ComposerInput[];
  layout: readonly LayoutSlot[];
  selectedInput?: string | null;
  className?: string;
  /** Render order: index 0 = back, last = front. */
  zOrder?: readonly string[];
  hideCaption?: boolean;
  loading?: boolean;
}

// Pure read-only SVG renderer for a composer canvas. Editors layer interaction
// on top via absolute-positioned overlays (see CanvasEditor / LayoutSlotHandle).
export function CanvasPreview({
  canvas,
  inputs,
  layout,
  selectedInput = null,
  className = '',
  zOrder,
  hideCaption = false,
  loading = false,
}: Readonly<CanvasPreviewProps>) {
  const aspectRatio = canvas.w / Math.max(1, canvas.h);

  // Map ref → effect badge text for label rendering.
  const inputByRef = new Map<string, ComposerInput>();
  for (const input of inputs) {
    inputByRef.set(input.ref, input);
  }

  // Resolve z-order: explicit override, else layout array order.
  const orderedSlots: LayoutSlot[] = (() => {
    if (!zOrder || zOrder.length === 0) return [...layout];
    const byInput = new Map(layout.map((s) => [s.input, s]));
    const out: LayoutSlot[] = [];
    for (const ref of zOrder) {
      const s = byInput.get(ref);
      if (s) out.push(s);
    }
    // Trailing un-ordered slots stay at the back of the explicit order.
    for (const s of layout) {
      if (!zOrder.includes(s.input)) out.unshift(s);
    }
    return out;
  })();

  return (
    <div className={className}>
      <div
        className="relative w-full border-2 border-border rounded-md bg-surface-sunken"
        style={{ aspectRatio: `${aspectRatio}` }}
      >
        {layout.length === 0 ? (
          <div className="absolute inset-0 flex items-center justify-center text-fg-subtle text-sm">
            {loading ? 'Loading layout…' : 'No layout slots'}
          </div>
        ) : (
          <svg
            viewBox={`0 0 ${canvas.w} ${canvas.h}`}
            preserveAspectRatio="xMidYMid meet"
            className="absolute inset-0 w-full h-full"
          >
            {orderedSlots.map((slot) => {
              const input = inputByRef.get(slot.input);
              const isSelected = selectedInput === slot.input;
              const labelSize = Math.max(
                14,
                Math.min(slot.w, slot.h) / 10,
              );
              const stroke = Math.max(2, canvas.w / 800);
              const fill = isSelected
                ? 'rgba(59, 130, 246, 0.30)'
                : 'rgba(59, 130, 246, 0.12)';
              const strokeColor = isSelected ? '#3b82f6' : '#64748b';
              const effectSuffix = input?.effect ? ` · ${input.effect.type}` : '';
              return (
                <g key={slot.input}>
                  <rect
                    x={slot.x}
                    y={slot.y}
                    width={slot.w}
                    height={slot.h}
                    fill={fill}
                    stroke={strokeColor}
                    strokeWidth={isSelected ? stroke * 2 : stroke}
                  />
                  <text
                    x={slot.x + slot.w / 2}
                    y={slot.y + slot.h / 2}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="#ffffff"
                    fontSize={labelSize}
                    fontFamily="monospace"
                  >
                    {slot.input}
                    {effectSuffix}
                  </text>
                </g>
              );
            })}
          </svg>
        )}
        {loading && layout.length > 0 && (
          <div className="absolute top-2 right-2 text-xs text-fg-subtle bg-surface-sunken/80 px-2 py-1 rounded">
            saving…
          </div>
        )}
      </div>
      {!hideCaption && (
        <p className="mt-2 text-xs text-fg-subtle text-center">
          {canvas.w}×{canvas.h} canvas · {layout.length} slot{layout.length === 1 ? '' : 's'}
        </p>
      )}
    </div>
  );
}

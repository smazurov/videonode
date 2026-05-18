import type { components } from '../lib/api.generated';

type CanvasLayoutData = components['schemas']['CanvasLayoutData'];

interface CanvasPreviewProps {
  canvasW: number;
  canvasH: number;
  layout: CanvasLayoutData | null;
  loading?: boolean;
  className?: string;
  onCycle?: () => void;
  chosenLayout?: string;
  availableCount?: number;
  hideCaption?: boolean;
}

export function CanvasPreview({
  canvasW,
  canvasH,
  layout,
  loading = false,
  className = '',
  onCycle,
  chosenLayout,
  availableCount,
  hideCaption = false,
}: Readonly<CanvasPreviewProps>) {
  const aspectRatio = canvasW / canvasH;
  const slots = layout?.slots ?? [];
  const cycleable = !!onCycle && (availableCount ?? 0) > 1;

  const handleKey = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!cycleable) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onCycle?.();
    }
  };

  return (
    <div className={className}>
      <div
        className={`relative w-full border-2 border-border rounded-md bg-surface-sunken ${
          cycleable ? 'cursor-pointer hover:ring-2 hover:ring-focus-ring focus-visible:ring-2 focus-visible:ring-focus-ring outline-none transition-shadow' : ''
        }`}
        style={{ aspectRatio: `${aspectRatio}` }}
        {...(cycleable
          ? {
              role: 'button',
              tabIndex: 0,
              onClick: onCycle,
              onKeyDown: handleKey,
              'aria-label': 'Click to cycle canvas layout',
            }
          : {})}
      >
        {slots.length === 0 ? (
          <div className="absolute inset-0 flex items-center justify-center text-fg-subtle text-sm">
            {loading ? 'Computing layout...' : 'Select source streams to preview layout'}
          </div>
        ) : (
          <svg
            viewBox={`0 0 ${canvasW} ${canvasH}`}
            preserveAspectRatio="xMidYMid meet"
            className="absolute inset-0 w-full h-full"
          >
            {slots.map((slot, i) => {
              const slotStroke = Math.max(3, canvasW / 600);
              const contentFill = 'rgba(59, 130, 246, 0.35)';
              const slotFill = 'rgba(59, 130, 246, 0.08)';
              const labelSize = Math.max(12, Math.min(slot.content_w, slot.content_h) / 8);
              const rotationSuffix = slot.rotation_applied !== 0 ? ` · ${slot.rotation_applied}°` : '';
              return (
                <g key={`${slot.source_stream_id}-${i}`}>
                  {/* Slot rectangle — shows where the layout solver placed this source. */}
                  <rect
                    x={slot.slot_x}
                    y={slot.slot_y}
                    width={slot.slot_w}
                    height={slot.slot_h}
                    fill={slotFill}
                    stroke="#64748b"
                    strokeDasharray={`${slotStroke * 2} ${slotStroke * 2}`}
                    strokeWidth={slotStroke}
                  />
                  {/* Content rectangle — where the letterboxed input pixels land. */}
                  <rect
                    x={slot.content_x}
                    y={slot.content_y}
                    width={slot.content_w}
                    height={slot.content_h}
                    fill={contentFill}
                    stroke="#3b82f6"
                    strokeWidth={Math.max(4, canvasW / 400)}
                  />
                  <text
                    x={slot.content_x + slot.content_w / 2}
                    y={slot.content_y + slot.content_h / 2}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="#ffffff"
                    fontSize={labelSize}
                    fontFamily="monospace"
                  >
                    {slot.source_stream_id}
                    {rotationSuffix}
                  </text>
                </g>
              );
            })}
          </svg>
        )}
        {loading && slots.length > 0 && (
          <div className="absolute top-2 right-2 text-xs text-fg-subtle bg-surface-sunken/80 px-2 py-1 rounded">
            updating...
          </div>
        )}
      </div>
      {!hideCaption && (
        <p className="mt-2 text-xs text-fg-subtle text-center">
          {canvasW}×{canvasH} canvas · {slots.length} source{slots.length === 1 ? '' : 's'}
          {chosenLayout ? ` · ${chosenLayout}` : ''}
          {cycleable ? ' · click to cycle' : ''}
        </p>
      )}
    </div>
  );
}

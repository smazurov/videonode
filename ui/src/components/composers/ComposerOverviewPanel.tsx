import { Card } from '../Card';
import type { ComposerData } from '../../lib/composer-types';
import { canvasFpsOrDefault, formatCanvasDims } from '../../lib/composer-types';

// Short label for SVG layout slots — full source refs like
// `source:usb-046d-hd-pro-webcam-c920-d6ba64df-video-index0` overflow
// any sensible thumbnail; strip the prefix and clip to ~14 chars.
function slotLabel(ref: string): string {
  const stripped = ref.startsWith('source:') ? ref.slice('source:'.length) : ref;
  return stripped.length > 14 ? stripped.slice(0, 13) + '…' : stripped;
}

interface ComposerOverviewPanelProps {
  composer: ComposerData;
}

// Small inline preview — a scaled SVG of the composer's static layout
// slots. Live preview (snapshot/WebRTC) belongs in U9's CanvasEditor.
function CanvasPreviewThumbnail({ composer }: Readonly<{ composer: ComposerData }>) {
  const { w, h } = composer.canvas;
  if (w <= 0 || h <= 0) {
    return (
      <div className="rounded-md border border-border bg-surface-sunken p-4 text-center text-xs text-fg-subtle">
        invalid canvas dims
      </div>
    );
  }
  const aspect = w / h;

  return (
    <div
      className="relative w-full max-w-sm overflow-hidden rounded-md border border-border bg-surface-sunken"
      style={{ aspectRatio: aspect }}
    >
      <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="xMidYMid meet" className="absolute inset-0 h-full w-full">
        {composer.layout.map((slot, idx) => (
          <g key={`${slot.input}-${idx}`}>
            <rect
              x={slot.x}
              y={slot.y}
              width={slot.w}
              height={slot.h}
              fill="rgba(59, 130, 246, 0.25)"
              stroke="#3b82f6"
              strokeWidth={Math.max(2, w / 480)}
            />
            <text
              x={slot.x + slot.w / 2}
              y={slot.y + slot.h / 2}
              textAnchor="middle"
              dominantBaseline="middle"
              fill="#ffffff"
              fontSize={Math.max(14, Math.min(slot.w, slot.h) / 8)}
              fontFamily="monospace"
              lengthAdjust="spacingAndGlyphs"
              textLength={Math.min(slot.w * 0.9, slotLabel(slot.input).length * Math.max(14, Math.min(slot.w, slot.h) / 8) * 0.6)}
            >
              <title>{slot.input}</title>
              {slotLabel(slot.input)}
            </text>
          </g>
        ))}
      </svg>
    </div>
  );
}

export function ComposerOverviewPanel({ composer }: Readonly<ComposerOverviewPanelProps>) {
  return (
    <Card>
      <Card.Header>
        <h2 className="text-sm font-semibold text-fg">Overview</h2>
      </Card.Header>
      <Card.Content>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
          <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <dt className="text-fg-muted">Composer ID</dt>
            <dd className="font-mono">{composer.composer_id}</dd>
            <dt className="text-fg-muted">Canvas</dt>
            <dd className="font-mono">{formatCanvasDims(composer.canvas)}</dd>
            <dt className="text-fg-muted">Frame rate</dt>
            <dd className="font-mono tabular-nums">
              {canvasFpsOrDefault(composer.canvas)} fps
              {!composer.canvas.fps && <span className="ml-1 text-fg-subtle">(default)</span>}
            </dd>
            <dt className="text-fg-muted">Inputs</dt>
            <dd className="tabular-nums">{composer.inputs.length}</dd>
            <dt className="text-fg-muted">Layout slots</dt>
            <dd className="tabular-nums">{composer.layout.length}</dd>
          </dl>
          <div className="flex-1">
            <CanvasPreviewThumbnail composer={composer} />
          </div>
        </div>
      </Card.Content>
    </Card>
  );
}

import { useCallback } from 'react';

export type HandlePos =
  | 'nw'
  | 'n'
  | 'ne'
  | 'e'
  | 'se'
  | 's'
  | 'sw'
  | 'w'
  | 'move';

interface LayoutSlotHandleProps {
  position: HandlePos;
  /** Pointer-down handler receives the originating event; CanvasEditor sets up move tracking. */
  onPointerDown: (e: React.PointerEvent<HTMLDivElement>, position: HandlePos) => void;
}

const CURSORS: Record<HandlePos, string> = {
  nw: 'nwse-resize',
  n: 'ns-resize',
  ne: 'nesw-resize',
  e: 'ew-resize',
  se: 'nwse-resize',
  s: 'ns-resize',
  sw: 'nesw-resize',
  w: 'ew-resize',
  move: 'move',
};

// Absolute positioning relative to the slot's bounding box (in editor px).
// `move` is the slot-body grab handle; other 8 are corner / edge resize.
const HANDLE_OFFSETS: Record<Exclude<HandlePos, 'move'>, { top: string; left: string }> = {
  nw: { top: '0%', left: '0%' },
  n: { top: '0%', left: '50%' },
  ne: { top: '0%', left: '100%' },
  e: { top: '50%', left: '100%' },
  se: { top: '100%', left: '100%' },
  s: { top: '100%', left: '50%' },
  sw: { top: '100%', left: '0%' },
  w: { top: '50%', left: '0%' },
};

const HANDLE_SIZE = 10;

export function LayoutSlotHandle({ position, onPointerDown }: Readonly<LayoutSlotHandleProps>) {
  const handlePointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      e.stopPropagation();
      onPointerDown(e, position);
    },
    [onPointerDown, position],
  );

  if (position === 'move') {
    // The "move" handle is the slot body itself — rendered as a transparent
    // overlay covering the slot rect. CanvasEditor places it; this component
    // just attaches the pointer handler.
    return (
      <div
        className="absolute inset-0"
        style={{ cursor: CURSORS.move }}
        onPointerDown={handlePointerDown}
        role="presentation"
      />
    );
  }

  const offset = HANDLE_OFFSETS[position];
  return (
    <div
      className="absolute bg-accent border border-accent-fg rounded-sm shadow-sm"
      style={{
        width: HANDLE_SIZE,
        height: HANDLE_SIZE,
        top: offset.top,
        left: offset.left,
        transform: 'translate(-50%, -50%)',
        cursor: CURSORS[position],
        touchAction: 'none',
      }}
      onPointerDown={handlePointerDown}
      role="presentation"
      aria-label={`Resize ${position}`}
    />
  );
}

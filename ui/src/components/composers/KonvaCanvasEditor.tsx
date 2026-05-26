import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { Stage, Layer, Rect, Text, Line, Transformer, Group } from 'react-konva';
import type Konva from 'konva';
import type { Box } from 'konva/lib/shapes/Transformer';

import { CanvasRuler } from './CanvasRuler';
import type {
  CanvasDims,
  ComposerInput,
  LayoutSlot,
} from '../../lib/composer-types';

interface KonvaCanvasEditorProps {
  canvas: CanvasDims;
  inputs: readonly ComposerInput[];
  layout: readonly LayoutSlot[];
  selectedInput: string | null;
  onSelect: (input: string | null) => void;
  onLayoutChange: (next: LayoutSlot[]) => void;
  gridSize?: number;
  snapToGrid?: boolean;
  showRulers?: boolean;
}

const MIN_SIZE = 16;
const ALIGN_THRESHOLD = 6;

function snapVal(v: number, grid: number): number {
  return Math.round(v / grid) * grid;
}

interface Guide {
  orientation: 'V' | 'H';
  pos: number;
}

function findAlignmentSnap(
  pos: number, size: number, candidates: number[],
): { snappedPos: number; guidePos: number } | null {
  for (const c of candidates) {
    if (Math.abs(pos - c) <= ALIGN_THRESHOLD) return { snappedPos: c, guidePos: c };
    if (Math.abs(pos + size - c) <= ALIGN_THRESHOLD) return { snappedPos: c - size, guidePos: c };
    if (Math.abs(pos + size / 2 - c) <= ALIGN_THRESHOLD) return { snappedPos: c - size / 2, guidePos: c };
  }
  return null;
}

export function KonvaCanvasEditor({
  canvas,
  inputs,
  layout,
  selectedInput,
  onSelect,
  onLayoutChange,
  gridSize = 10,
  snapToGrid = true,
  showRulers = true,
}: Readonly<KonvaCanvasEditorProps>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerW, setContainerW] = useState(0);
  const trRef = useRef<Konva.Transformer>(null);
  const shapeRefs = useRef<Map<string, Konva.Rect>>(new Map());
  const layerRef = useRef<Konva.Layer>(null);

  const layoutRef = useRef(layout);
  useEffect(() => {
    layoutRef.current = layout;
  }, [layout]);

  useLayoutEffect(() => {
    if (!containerRef.current) return;
    const el = containerRef.current;
    const ro = new ResizeObserver(() => setContainerW(el.clientWidth));
    ro.observe(el);
    setContainerW(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const aspect = canvas.w / Math.max(1, canvas.h);
  const stageW = containerW;
  const stageH = containerW / aspect;
  const scale = containerW / Math.max(1, canvas.w);

  const inputByRef = useMemo(() => {
    const m = new Map<string, ComposerInput>();
    for (const inp of inputs) m.set(inp.ref, inp);
    return m;
  }, [inputs]);

  const slotNumber = useMemo(
    () => new Map(layout.map((s, i) => [s.input, i + 1])),
    [layout],
  );

  // Attach transformer to selected node
  useEffect(() => {
    if (!trRef.current) return;
    if (!selectedInput) {
      trRef.current.nodes([]);
      return;
    }
    const node = shapeRefs.current.get(selectedInput);
    if (node) {
      trRef.current.nodes([node]);
    } else {
      trRef.current.nodes([]);
    }
  }, [selectedInput, layout]);

  const commitSlot = useCallback(
    (input: string, updates: Partial<LayoutSlot>) => {
      const next = layoutRef.current.map((s) =>
        s.input === input ? { ...s, ...updates } : s,
      );
      onLayoutChange(next);
    },
    [onLayoutChange],
  );

  const handleDragEnd = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      let x = e.target.x();
      let y = e.target.y();
      if (snapToGrid && gridSize > 0) {
        x = snapVal(x, gridSize);
        y = snapVal(y, gridSize);
        e.target.x(x);
        e.target.y(y);
      }
      commitSlot(slot.input, { x: Math.round(x), y: Math.round(y) });
    },
    [commitSlot, gridSize, snapToGrid],
  );

  const handleTransformEnd = useCallback(
    (slot: LayoutSlot) => {
      const node = shapeRefs.current.get(slot.input);
      if (!node) return;
      const scaleX = node.scaleX();
      const scaleY = node.scaleY();
      node.scaleX(1);
      node.scaleY(1);
      let w = Math.max(MIN_SIZE, Math.round(node.width() * scaleX));
      let h = Math.max(MIN_SIZE, Math.round(node.height() * scaleY));
      let x = Math.round(node.x());
      let y = Math.round(node.y());
      const rotation = Math.round(node.rotation() / 90) * 90;
      if (snapToGrid && gridSize > 0) {
        x = snapVal(x, gridSize);
        y = snapVal(y, gridSize);
        w = Math.max(MIN_SIZE, snapVal(w, gridSize));
        h = Math.max(MIN_SIZE, snapVal(h, gridSize));
      }
      if (x < 0) { w += x; x = 0; }
      if (y < 0) { h += y; y = 0; }
      if (x + w > canvas.w) w = canvas.w - x;
      if (y + h > canvas.h) h = canvas.h - y;
      w = Math.max(MIN_SIZE, w);
      h = Math.max(MIN_SIZE, h);
      node.x(x);
      node.y(y);
      node.width(w);
      node.height(h);
      node.rotation(rotation);
      commitSlot(slot.input, { x, y, w, h, rotation });
    },
    [canvas.h, canvas.w, commitSlot, gridSize, snapToGrid],
  );

  const makeDragBound = useCallback(
    (slotW: number, slotH: number) => (pos: Konva.Vector2d) => {
      let { x, y } = pos;
      if (snapToGrid && gridSize > 0) {
        x = snapVal(x, gridSize);
        y = snapVal(y, gridSize);
      }
      x = Math.max(0, Math.min(canvas.w - slotW, x));
      y = Math.max(0, Math.min(canvas.h - slotH, y));
      return { x, y };
    },
    [canvas.h, canvas.w, gridSize, snapToGrid],
  );

  const handleStageClick = useCallback(
    (e: Konva.KonvaEventObject<MouseEvent>) => {
      if (e.target.getParent() == null) {
        onSelect(null);
      }
    },
    [onSelect],
  );

  // Keyboard nudge
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!selectedInput) return;
      const tag = (e.target as HTMLElement | null)?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
      const step = e.shiftKey ? 10 : 1;
      const gridStep = snapToGrid && gridSize > 0 ? gridSize : step;
      let dx = 0;
      let dy = 0;
      switch (e.key) {
        case 'ArrowLeft':
          dx = -gridStep;
          break;
        case 'ArrowRight':
          dx = gridStep;
          break;
        case 'ArrowUp':
          dy = -gridStep;
          break;
        case 'ArrowDown':
          dy = gridStep;
          break;
        default:
          return;
      }
      e.preventDefault();
      const slot = layoutRef.current.find((s) => s.input === selectedInput);
      if (!slot) return;
      commitSlot(selectedInput, {
        x: Math.max(0, Math.min(canvas.w - slot.w, slot.x + dx)),
        y: Math.max(0, Math.min(canvas.h - slot.h, slot.y + dy)),
      });
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [canvas.h, canvas.w, commitSlot, gridSize, selectedInput, snapToGrid]);

  // Alignment guides state
  const [guides, setGuides] = useState<Guide[]>([]);

  const handleDragMove = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      const node = e.target;
      const newGuides: Guide[] = [];
      const vCands = [0, canvas.w / 2, canvas.w];
      const hCands = [0, canvas.h / 2, canvas.h];
      for (const other of layoutRef.current) {
        if (other.input === slot.input) continue;
        vCands.push(other.x, other.x + other.w, other.x + other.w / 2);
        hCands.push(other.y, other.y + other.h, other.y + other.h / 2);
      }
      const vSnap = findAlignmentSnap(node.x(), node.width(), vCands);
      if (vSnap) { node.x(vSnap.snappedPos); newGuides.push({ orientation: 'V', pos: vSnap.guidePos }); }
      const hSnap = findAlignmentSnap(node.y(), node.height(), hCands);
      if (hSnap) { node.y(hSnap.snappedPos); newGuides.push({ orientation: 'H', pos: hSnap.guidePos }); }
      setGuides(newGuides);
    },
    [canvas.h, canvas.w],
  );

  const handleDragEndWithGuides = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      setGuides([]);
      handleDragEnd(slot, e);
    },
    [handleDragEnd],
  );

  const boundBoxFunc = useCallback(
    (oldBox: Box, newBox: Box) => {
      if (Math.abs(newBox.width) < MIN_SIZE || Math.abs(newBox.height) < MIN_SIZE) {
        return oldBox;
      }
      return newBox;
    },
    [],
  );

  if (stageW <= 0) {
    return <div ref={containerRef} className="min-w-0" />;
  }

  return (
    <div className="select-none min-w-0">
      {showRulers && stageW > 0 && (
        <div className="flex items-start pl-5">
          <CanvasRuler size={canvas.w} displaySize={stageW} orientation="horizontal" />
        </div>
      )}
      <div className="flex items-start">
        {showRulers && stageH > 0 && (
          <CanvasRuler size={canvas.h} displaySize={stageH} orientation="vertical" />
        )}
        <div ref={containerRef} className="flex-1 min-w-0">
          <Stage
            width={stageW}
            height={stageH}
            scaleX={scale}
            scaleY={scale}
            onClick={handleStageClick}
            onTap={handleStageClick as never}
          >
            <Layer ref={layerRef}>
              {/* Canvas background */}
              <Rect
                x={0}
                y={0}
                width={canvas.w}
                height={canvas.h}
                fill="#0f172a"
                stroke="#334155"
                strokeWidth={2}
                listening={false}
              />
              {/* Slot rectangles */}
              {layout.map((slot) => {
                const isSelected = selectedInput === slot.input;
                const num = slotNumber.get(slot.input) ?? '?';
                const inp = inputByRef.get(slot.input);
                const effectSuffix = inp?.effect ? ` · ${inp.effect.type}` : '';
                const rotSuffix = slot.rotation ? ` ↻${slot.rotation}°` : '';
                const labelSize = Math.max(14, Math.min(slot.w, slot.h) / 10);
                return (
                  <Group
                    key={slot.input}
                    x={slot.x}
                    y={slot.y}
                    width={slot.w}
                    height={slot.h}
                    rotation={slot.rotation ?? 0}
                    draggable
                    ref={(node: Konva.Group | null) => {
                      if (node) shapeRefs.current.set(slot.input, node as unknown as Konva.Rect);
                      else shapeRefs.current.delete(slot.input);
                    }}
                    onClick={() => onSelect(slot.input)}
                    onTap={() => onSelect(slot.input)}
                    onDragMove={(e) => handleDragMove(slot, e)}
                    onDragEnd={(e) => handleDragEndWithGuides(slot, e)}
                    onTransformEnd={() => handleTransformEnd(slot)}
                    dragBoundFunc={makeDragBound(slot.w, slot.h)}
                  >
                    <Rect
                      width={slot.w}
                      height={slot.h}
                      fill={isSelected ? 'rgba(59, 130, 246, 0.30)' : 'rgba(59, 130, 246, 0.12)'}
                      stroke={isSelected ? '#3b82f6' : '#64748b'}
                      strokeWidth={isSelected ? 4 : 2}
                    />
                    <Text
                      text={`${num}${effectSuffix}${rotSuffix}`}
                      x={0}
                      y={0}
                      width={slot.w}
                      height={slot.h}
                      align="center"
                      verticalAlign="middle"
                      fill="#ffffff"
                      fontSize={labelSize}
                      fontFamily="monospace"
                    />
                  </Group>
                );
              })}
              {/* Alignment guides */}
              {guides.map((g, i) =>
                g.orientation === 'V' ? (
                  <Line
                    key={`g-${i}`}
                    points={[g.pos, 0, g.pos, canvas.h]}
                    stroke="#22c55e"
                    strokeWidth={1}
                    dash={[4, 3]}
                    listening={false}
                  />
                ) : (
                  <Line
                    key={`g-${i}`}
                    points={[0, g.pos, canvas.w, g.pos]}
                    stroke="#22c55e"
                    strokeWidth={1}
                    dash={[4, 3]}
                    listening={false}
                  />
                ),
              )}
              {/* Transformer */}
              <Transformer
                ref={trRef}
                rotationSnaps={[0, 90, 180, 270]}
                rotationSnapTolerance={45}
                keepRatio={true}
                flipEnabled={false}
                boundBoxFunc={boundBoxFunc}
                anchorCornerRadius={2}
                anchorStroke="#3b82f6"
                anchorFill="#ffffff"
                borderStroke="#3b82f6"
                borderStrokeWidth={2}
              />
            </Layer>
          </Stage>
        </div>
      </div>
    </div>
  );
}

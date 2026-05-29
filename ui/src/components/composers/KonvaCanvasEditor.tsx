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
import { useShallow } from 'zustand/shallow';

import { CanvasRuler } from './CanvasRuler';
import {
  clamp,
  computeArPreview,
  konvaToVisual,
  rotateOffset,
  snapVal,
  visualDims,
  visualToKonva,
} from './layout-math';
import type { ComposerInput, LayoutSlot } from '../../lib/composer-types';
import { useLayoutEditorStore } from '../../hooks/useLayoutEditorStore';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SourceDims { w: number; h: number }

interface Props {
  inputs: readonly ComposerInput[];
  sourceDims?: ReadonlyMap<string, SourceDims>;
  gridSize?: number;
  snapToGrid?: boolean;
  showRulers?: boolean;
  showGrid?: boolean;
}

interface GhostBox { x: number; y: number; w: number; h: number }
interface CropPan { startX: number; startY: number; cropX: number; cropY: number }

const MIN_SIZE = 16;
const ANCHORS_ALL = [
  'top-left', 'top-center', 'top-right',
  'middle-left', 'middle-right',
  'bottom-left', 'bottom-center', 'bottom-right',
] as const;
const ANCHORS_CORNERS = ['top-left', 'top-right', 'bottom-left', 'bottom-right'] as const;

// ---------------------------------------------------------------------------
// Helpers (pure, pulled out to reduce handler complexity)
// ---------------------------------------------------------------------------

function computeNormalDragResult(
  node: Konva.Node, slot: LayoutSlot,
  canvasW: number, canvasH: number,
  doSnap: boolean, gridSize: number,
) {
  const rot = slot.rotation ?? 0;
  const { vw, vh } = visualDims(slot.w, slot.h, rot);
  const vis = konvaToVisual(node.x(), node.y(), slot.w, slot.h, rot);
  if (doSnap && gridSize > 0) {
    vis.x = snapVal(vis.x, gridSize);
    vis.y = snapVal(vis.y, gridSize);
  }
  vis.x = clamp(Math.round(vis.x), 0, canvasW - vw);
  vis.y = clamp(Math.round(vis.y), 0, canvasH - vh);
  return { vis, kPos: visualToKonva(vis.x, vis.y, slot.w, slot.h, rot) };
}

function computeCropPanOffset(
  dx: number, dy: number, start: CropPan,
  slot: LayoutSlot, sd: SourceDims,
) {
  const fillScale = Math.max(slot.w / sd.w, slot.h / sd.h);
  const s = Math.max(1, slot.crop?.scale ?? 1);
  const scaledW = sd.w * fillScale * s;
  const scaledH = sd.h * fillScale * s;
  const exW = Math.max(1, scaledW - slot.w);
  const exH = Math.max(1, scaledH - slot.h);
  return {
    cx: clamp(start.cropX - dx / exW, 0, 1),
    cy: clamp(start.cropY - dy / exH, 0, 1),
  };
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function KonvaCanvasEditor({
  inputs, sourceDims, gridSize = 10, snapToGrid = true, showRulers = true, showGrid = false,
}: Readonly<Props>) {
  // Store
  const { canvas, layout, selectedInput } = useLayoutEditorStore(
    useShallow((s) => ({ canvas: s.canvas, layout: s.layout, selectedInput: s.selectedInput })),
  );
  const { commitSlot, select: onSelect } = useLayoutEditorStore(
    useShallow((s) => ({ commitSlot: s.commitSlot, select: s.select })),
  );

  // Container
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerW, setContainerW] = useState(0);

  // Konva refs
  const trRef = useRef<Konva.Transformer>(null);
  const layerRef = useRef<Konva.Layer>(null);
  const outputRects = useRef<Map<string, Konva.Rect>>(new Map());
  const slotGroups = useRef<Map<string, Konva.Group>>(new Map());
  const cropSrcRects = useRef<Map<string, Konva.Rect>>(new Map());

  // Shift key (ref = synchronous, state = re-render for Transformer switch)
  const shiftRef = useRef(false);
  const [shiftHeld, setShiftHeld] = useState(false);
  useEffect(() => {
    const d = (e: KeyboardEvent) => { if (e.key === 'Shift') { shiftRef.current = true; setShiftHeld(true); } };
    const u = (e: KeyboardEvent) => { if (e.key === 'Shift') { shiftRef.current = false; setShiftHeld(false); } };
    window.addEventListener('keydown', d);
    window.addEventListener('keyup', u);
    return () => { window.removeEventListener('keydown', d); window.removeEventListener('keyup', u); };
  }, []);

  // Crop pan drag state
  const cropPanRef = useRef<CropPan | null>(null);
  const [liveCrop, setLiveCrop] = useState<{ input: string; cx: number; cy: number } | null>(null);

  // Ghost box
  const [ghostBox, setGhostBox] = useState<GhostBox | null>(null);

  // Container resize observer
  useLayoutEffect(() => {
    if (!containerRef.current) return;
    const el = containerRef.current;
    const ro = new ResizeObserver(() => setContainerW(el.clientWidth));
    ro.observe(el);
    setContainerW(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  // Derived measurements
  const stageW = containerW;
  const stageH = containerW / Math.max(1, canvas.w / Math.max(1, canvas.h));
  const scale = containerW / Math.max(1, canvas.w);

  // Lookup maps
  const inputByRef = useMemo(() => {
    const m = new Map<string, ComposerInput>();
    for (const inp of inputs) m.set(inp.ref, inp);
    return m;
  }, [inputs]);

  const slotNumber = useMemo(
    () => new Map(layout.map((s, i) => [s.input, i + 1])),
    [layout],
  );

  // ---------------------------------------------------------------------------
  // Transformer attachment
  // ---------------------------------------------------------------------------

  useEffect(() => {
    if (!trRef.current) return;
    if (!selectedInput) { trRef.current.nodes([]); return; }
    const slot = layout.find((s) => s.input === selectedInput);
    const useSrc = shiftHeld && slot?.aspect_ratio_mode === 'crop';
    const node = useSrc
      ? cropSrcRects.current.get(selectedInput)
      : outputRects.current.get(selectedInput);
    trRef.current.nodes(node ? [node] : []);
  }, [selectedInput, layout, shiftHeld]);

  // ---------------------------------------------------------------------------
  // Drag handlers
  // ---------------------------------------------------------------------------

  const handleDragStart = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      cropPanRef.current = shiftRef.current && slot.aspect_ratio_mode === 'crop'
        ? { startX: e.target.x(), startY: e.target.y(), cropX: slot.crop?.x ?? 0.5, cropY: slot.crop?.y ?? 0.5 }
        : null;
    },
    [],
  );

  const doCropPanMove = useCallback(
    (slot: LayoutSlot, node: Konva.Node) => {
      const start = cropPanRef.current!;
      const dx = node.x() - start.startX;
      const dy = node.y() - start.startY;
      node.x(start.startX);
      node.y(start.startY);
      const sd = sourceDims?.get(slot.input);
      if (!sd) return;
      const { cx, cy } = computeCropPanOffset(dx, dy, start, slot, sd);
      setLiveCrop({ input: slot.input, cx, cy });
    },
    [sourceDims],
  );

  const doNormalDragMove = useCallback(
    (slot: LayoutSlot, node: Konva.Node) => {
      const rot = slot.rotation ?? 0;
      const { vw, vh } = visualDims(slot.w, slot.h, rot);
      const vis = konvaToVisual(node.x(), node.y(), slot.w, slot.h, rot);

      const kPos = visualToKonva(vis.x, vis.y, slot.w, slot.h, rot);
      node.x(kPos.x);
      node.y(kPos.y);

      let gx = vis.x, gy = vis.y;
      if (snapToGrid && gridSize > 0) {
        gx = snapVal(gx, gridSize);
        gy = snapVal(gy, gridSize);
      }
      setGhostBox({
        x: clamp(Math.round(gx), 0, canvas.w - vw),
        y: clamp(Math.round(gy), 0, canvas.h - vh),
        w: vw, h: vh,
      });
    },
    [canvas.h, canvas.w, gridSize, snapToGrid],
  );

  const handleDragMove = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      if (cropPanRef.current) doCropPanMove(slot, e.target);
      else doNormalDragMove(slot, e.target);
    },
    [doCropPanMove, doNormalDragMove],
  );

  const handleDragEnd = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      setGhostBox(null);

      if (cropPanRef.current) {
        if (liveCrop?.input === slot.input) {
          commitSlot(slot.input, {
            crop: {
              ...slot.crop ?? { x: 0.5, y: 0.5, scale: 1 },
              x: Math.round(liveCrop.cx * 100) / 100,
              y: Math.round(liveCrop.cy * 100) / 100,
            },
          });
        }
        cropPanRef.current = null;
        setLiveCrop(null);
        return;
      }

      const { vis, kPos } = computeNormalDragResult(
        e.target, slot, canvas.w, canvas.h, snapToGrid, gridSize,
      );
      e.target.x(kPos.x);
      e.target.y(kPos.y);
      commitSlot(slot.input, { x: vis.x, y: vis.y });
    },
    [canvas.h, canvas.w, commitSlot, gridSize, liveCrop, snapToGrid],
  );

  // ---------------------------------------------------------------------------
  // Transform handlers
  // ---------------------------------------------------------------------------

  const handleOutputTransformEnd = useCallback(
    (slot: LayoutSlot) => {
      const rect = outputRects.current.get(slot.input);
      const group = slotGroups.current.get(slot.input);
      if (!rect || !group) return;

      const sx = rect.scaleX(), sy = rect.scaleY();
      rect.scaleX(1); rect.scaleY(1);

      let w = Math.max(MIN_SIZE, Math.round(rect.width() * sx));
      let h = Math.max(MIN_SIZE, Math.round(rect.height() * sy));
      const rotation = Math.round(group.rotation() / 90) * 90;
      const doSnap = snapToGrid && gridSize > 0;
      if (doSnap) {
        w = Math.max(MIN_SIZE, snapVal(w, gridSize));
        h = Math.max(MIN_SIZE, snapVal(h, gridSize));
      }

      const offX = rect.x(), offY = rect.y();
      rect.x(0); rect.y(0);

      const { vw, vh } = visualDims(w, h, rotation);
      // offX/offY live in the group's local (unrotated) frame; rotate them into
      // world space before adding to the group origin, else a rotated resize jumps.
      const { dx, dy } = rotateOffset(offX, offY, rotation);
      const vis = konvaToVisual(group.x() + dx, group.y() + dy, w, h, rotation);
      if (doSnap) { vis.x = snapVal(vis.x, gridSize); vis.y = snapVal(vis.y, gridSize); }
      vis.x = clamp(Math.round(vis.x), 0, canvas.w - vw);
      vis.y = clamp(Math.round(vis.y), 0, canvas.h - vh);

      const kPos = visualToKonva(vis.x, vis.y, w, h, rotation);
      group.x(kPos.x); group.y(kPos.y);
      rect.width(w); rect.height(h);
      group.rotation(rotation);

      const updates: Partial<LayoutSlot> = { x: vis.x, y: vis.y, w, h, rotation };
      if (slot.aspect_ratio_mode === 'crop') {
        const base = slot.crop ?? { x: 0.5, y: 0.5, scale: 1 };
        let cx = base.x;
        let cy = base.y;
        if (w !== slot.w) cx = vis.x > slot.x + 5 ? 1 : 0;
        if (h !== slot.h) cy = vis.y > slot.y + 5 ? 1 : 0;
        updates.crop = { ...base, x: cx, y: cy };
      }
      commitSlot(slot.input, updates);
    },
    [canvas.h, canvas.w, commitSlot, gridSize, snapToGrid],
  );

  const handleCropSrcTransformEnd = useCallback(
    (slot: LayoutSlot) => {
      const srcRect = cropSrcRects.current.get(slot.input);
      const sd = sourceDims?.get(slot.input);
      if (!srcRect || !sd) return;

      const t = srcRect.scaleX();
      srcRect.scaleX(1); srcRect.scaleY(1);

      const srcAr = sd.w / sd.h;
      const rawW = srcRect.width() * t;
      const w1 = Math.max(slot.w, rawW);
      const h1 = w1 / srcAr;
      const fw = h1 >= slot.h ? w1 : slot.h * srcAr;
      const fh = fw / srcAr;

      const cx = fw > slot.w ? clamp(-srcRect.x() / (fw - slot.w), 0, 1) : 0.5;
      const cy = fh > slot.h ? clamp(-srcRect.y() / (fh - slot.h), 0, 1) : 0.5;

      const fillScale = Math.max(slot.w / sd.w, slot.h / sd.h);
      const newScale = Math.max(1, fw / (sd.w * fillScale));

      const preview = computeArPreview(slot.w, slot.h, sd.w, sd.h, 'crop', cx, cy, newScale);
      if (preview) {
        srcRect.x(preview.x); srcRect.y(preview.y);
        srcRect.width(preview.w); srcRect.height(preview.h);
      }
      commitSlot(slot.input, {
        crop: {
          x: Math.round(cx * 100) / 100,
          y: Math.round(cy * 100) / 100,
          scale: Math.round(newScale * 100) / 100,
        },
      });
    },
    [commitSlot, sourceDims],
  );

  // ---------------------------------------------------------------------------
  // Stage deselect + keyboard nudge
  // ---------------------------------------------------------------------------

  const handleStageInteract = useMemo(
    () => (e: Konva.KonvaEventObject<MouseEvent | TouchEvent>) => {
      if (!e.target.getParent()) onSelect(null);
    },
    [onSelect],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!selectedInput) return;
      const tag = (e.target as HTMLElement | null)?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
      const baseStep = e.shiftKey ? 10 : 1;
      const step = snapToGrid && gridSize > 0 ? gridSize : baseStep;
      let dx = 0, dy = 0;
      switch (e.key) {
        case 'ArrowLeft': dx = -step; break;
        case 'ArrowRight': dx = step; break;
        case 'ArrowUp': dy = -step; break;
        case 'ArrowDown': dy = step; break;
        default: return;
      }
      e.preventDefault();
      const slot = useLayoutEditorStore.getState().layout.find((s) => s.input === selectedInput);
      if (!slot) return;
      const { vw, vh } = visualDims(slot.w, slot.h, slot.rotation ?? 0);
      commitSlot(selectedInput, {
        x: clamp(slot.x + dx, 0, canvas.w - vw),
        y: clamp(slot.y + dy, 0, canvas.h - vh),
      });
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [canvas.h, canvas.w, commitSlot, gridSize, selectedInput, snapToGrid]);

  // ---------------------------------------------------------------------------
  // boundBoxFunc
  // ---------------------------------------------------------------------------

  const boundBoxFunc = useCallback(
    (oldBox: Box, newBox: Box) => {
      const minW = MIN_SIZE * scale, minH = MIN_SIZE * scale;
      if (Math.abs(newBox.width) < minW || Math.abs(newBox.height) < minH) return oldBox;
      const maxW = canvas.w * scale, maxH = canvas.h * scale;
      const c = { ...newBox };
      if (c.x < 0) { c.width += c.x; c.x = 0; }
      if (c.y < 0) { c.height += c.y; c.y = 0; }
      if (c.x + c.width > maxW) c.width = maxW - c.x;
      if (c.y + c.height > maxH) c.height = maxH - c.y;
      if (c.width < minW || c.height < minH) return oldBox;
      return c;
    },
    [canvas.h, canvas.w, scale],
  );

  // ---------------------------------------------------------------------------
  // Grid lines
  // ---------------------------------------------------------------------------

  const gridLines = useMemo(() => {
    if (!showGrid || gridSize <= 0) return [];
    const out: number[][] = [];
    for (let x = gridSize; x < canvas.w; x += gridSize) out.push([x, 0, x, canvas.h]);
    for (let y = gridSize; y < canvas.h; y += gridSize) out.push([0, y, canvas.w, y]);
    return out;
  }, [canvas.h, canvas.w, gridSize, showGrid]);

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (stageW <= 0) return <div ref={containerRef} className="min-w-0" />;

  return (
    <div className="select-none min-w-0 overflow-hidden">
      {showRulers && stageW > 0 && (
        <div className="flex items-start pl-7">
          <CanvasRuler size={canvas.w} displaySize={stageW} orientation="horizontal" />
        </div>
      )}
      <div className="flex items-start min-w-0">
        {showRulers && stageH > 0 && (
          <CanvasRuler size={canvas.h} displaySize={stageH} orientation="vertical" />
        )}
        <div ref={containerRef} className="flex-1 min-w-0 ml-px">
          <Stage width={stageW} height={stageH} scaleX={scale} scaleY={scale}
            onClick={handleStageInteract} onTap={handleStageInteract}>
            <Layer ref={layerRef}>
              {/* Background */}
              <Rect x={0} y={0} width={canvas.w} height={canvas.h}
                fill="#0f172a" stroke="#334155" strokeWidth={2} listening={false} />

              {/* Grid */}
              {gridLines.map((pts, i) => (
                <Line key={`gl${i}`} points={pts}
                  stroke="rgba(255,255,255,0.06)" strokeWidth={1} listening={false} />
              ))}

              {/* Ghost */}
              {ghostBox && (
                <Rect x={ghostBox.x} y={ghostBox.y} width={ghostBox.w} height={ghostBox.h}
                  fill="rgba(59,130,246,0.06)" stroke="rgba(59,130,246,0.4)"
                  strokeWidth={2} dash={[8, 4]} listening={false} />
              )}

              {/* Slots */}
              {layout.map((slot) => {
                const isSelected = selectedInput === slot.input;
                const rot = slot.rotation ?? 0;
                const kPos = visualToKonva(slot.x, slot.y, slot.w, slot.h, rot);
                const sd = sourceDims?.get(slot.input);
                const arMode = slot.aspect_ratio_mode;
                const inp = inputByRef.get(slot.input);

                const override = liveCrop?.input === slot.input ? liveCrop : null;
                const arPreview = sd
                  ? computeArPreview(slot.w, slot.h, sd.w, sd.h, arMode,
                      override?.cx ?? slot.crop?.x, override?.cy ?? slot.crop?.y, slot.crop?.scale)
                  : null;
                const isCrop = Boolean(arPreview && arMode === 'crop');

                const num = slotNumber.get(slot.input) ?? '?';
                const eff = inp?.effect ? ` · ${inp.effect.type}` : '';
                const rotL = rot ? ` ↻${rot}°` : '';
                let arL = '';
                if (arMode === 'fit') arL = ' [fit]';
                else if (arMode === 'crop') arL = ' [crop]';
                const fontSize = Math.max(14, Math.min(slot.w, slot.h) / 10);

                // Crop overlay elements (rendered behind output rect, inside same Group)
                let cropOverlay: React.ReactNode = null;
                if (isCrop && arPreview) {
                  const { x: sx, y: sy, w: sw, h: sh } = arPreview;
                  const topH = Math.max(0, -sy);
                  const botStart = slot.h - sy;
                  const botH = Math.max(0, sh - botStart);
                  const midTop = Math.max(0, -sy);
                  const midH = Math.min(sh, slot.h - sy) - midTop;
                  const leftW = Math.max(0, -sx);
                  const rightStart = slot.w - sx;
                  const rightW = Math.max(0, sw - rightStart);

                  cropOverlay = (
                    <Group key={`crop-${slot.input}`}>
                      <Rect
                        ref={(n: Konva.Rect | null) => {
                          if (n) cropSrcRects.current.set(slot.input, n);
                          else cropSrcRects.current.delete(slot.input);
                        }}
                        x={sx} y={sy} width={sw} height={sh}
                        fill="transparent"
                        stroke="rgba(34,197,94,0.6)" strokeWidth={4} dash={[8, 4]}
                        onTransformEnd={() => handleCropSrcTransformEnd(slot)}
                      />
                      {topH > 0 && <Rect x={sx} y={sy} width={sw} height={topH} fill="rgba(139,0,0,0.45)" />}
                      {botH > 0 && <Rect x={sx} y={sy + botStart} width={sw} height={botH} fill="rgba(139,0,0,0.45)" />}
                      {leftW > 0 && midH > 0 && <Rect x={sx} y={sy + midTop} width={leftW} height={midH} fill="rgba(139,0,0,0.45)" />}
                      {rightW > 0 && midH > 0 && <Rect x={sx + rightStart} y={sy + midTop} width={rightW} height={midH} fill="rgba(139,0,0,0.45)" />}
                    </Group>
                  );
                }

                return (
                  <Group key={slot.input}
                    x={kPos.x} y={kPos.y} width={slot.w} height={slot.h} rotation={rot}
                    draggable
                    ref={(n: Konva.Group | null) => {
                      if (n) slotGroups.current.set(slot.input, n);
                      else slotGroups.current.delete(slot.input);
                    }}
                    onClick={() => onSelect(slot.input)}
                    onTap={() => onSelect(slot.input)}
                    onDragStart={(e) => handleDragStart(slot, e)}
                    onDragMove={(e) => handleDragMove(slot, e)}
                    onDragEnd={(e) => handleDragEnd(slot, e)}
                  >
                    {cropOverlay}
                    <Rect
                      ref={(n: Konva.Rect | null) => {
                        if (n) outputRects.current.set(slot.input, n);
                        else outputRects.current.delete(slot.input);
                      }}
                      width={slot.w} height={slot.h}
                      fill={isSelected ? 'rgba(239,68,68,0.18)' : 'rgba(59,130,246,0.12)'}
                      stroke={isSelected ? '#ef4444' : '#64748b'}
                      strokeWidth={isSelected ? 3 : 2}
                      onTransformEnd={() => handleOutputTransformEnd(slot)}
                    />
                    {arPreview && !isCrop && (
                      <Rect x={arPreview.x} y={arPreview.y} width={arPreview.w} height={arPreview.h}
                        fill="rgba(34,197,94,0.10)" stroke="rgba(34,197,94,0.5)"
                        strokeWidth={2} dash={[6, 3]} listening={false} />
                    )}
                    <Text text={`${num}${eff}${rotL}${arL}`}
                      x={0} y={0} width={slot.w} height={slot.h}
                      align="center" verticalAlign="middle"
                      fill="#ffffff" fontSize={fontSize} fontFamily="monospace" />
                  </Group>
                );
              })}

              {/* Transformer */}
              <Transformer ref={trRef}
                rotateEnabled={false}
                keepRatio={true}
                shiftBehavior="none"
                enabledAnchors={shiftHeld ? [...ANCHORS_CORNERS] : [...ANCHORS_ALL]}
                flipEnabled={false}
                boundBoxFunc={boundBoxFunc}
                anchorSize={10} anchorCornerRadius={2}
                anchorStroke={shiftHeld ? '#22c55e' : '#3b82f6'} anchorFill="#ffffff"
                borderStroke={shiftHeld ? '#22c55e' : '#3b82f6'} borderStrokeWidth={2}
              />
            </Layer>
          </Stage>
        </div>
      </div>
    </div>
  );
}

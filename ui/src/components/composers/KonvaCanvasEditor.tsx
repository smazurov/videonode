import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Stage,
  Layer,
  Rect,
  Text,
  Line,
  Transformer,
  Group,
} from "react-konva";
import type Konva from "konva";
import type { Box } from "konva/lib/shapes/Transformer";
import { useShallow } from "zustand/shallow";

import { CanvasRuler } from "./CanvasRuler";
import {
  type ContentTransform,
  type RegionRef,
  type ResizeHandle,
  MIN_REGION_SIZE,
  backendToContent,
  clampRegionMove,
  collectAlignmentCandidates,
  computeContentPlacement,
  findAlignmentSnap,
  panContentByPixels,
  resizeRegion,
  snapVal,
} from "./region-content";
import type { ComposerInput, LayoutSlot } from "../../lib/composer-types";
import { semanticTokens } from "../../design/tokens";
import { useLayoutEditorStore } from "../../hooks/useLayoutEditorStore";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SourceDims {
  w: number;
  h: number;
}

interface Props {
  inputs: readonly ComposerInput[];
  sourceDims?: ReadonlyMap<string, SourceDims>;
  gridSize?: number;
  snapToGrid?: boolean;
  showRulers?: boolean;
  showGrid?: boolean;
}

interface Guides {
  v: number[];
  h: number[];
}
interface PanStart {
  startX: number;
  startY: number;
  content: ContentTransform;
}

const DEFAULT_ANCHOR: ResizeHandle = "bottom-right";
// A corner anchor moves on both axes (has a horizontal AND a vertical part).
const isCornerAnchor = (a: string): boolean =>
  (a.includes("left") || a.includes("right")) &&
  (a.includes("top") || a.includes("bottom"));

// Source dimensions for a slot, falling back to the region's own AR when the
// real source size is unknown (so cover/fit degenerate to filling the region).
function srcDimsFor(
  sourceDims: ReadonlyMap<string, SourceDims> | undefined,
  slot: LayoutSlot,
): SourceDims {
  return sourceDims?.get(slot.input) ?? { w: slot.w, h: slot.h };
}

// Konva paints to a <canvas>, where CSS `var(--color-…)` does not resolve — so
// we read the resolved values of the design-system tokens once (theme-aware),
// falling back to the generated token table when there is no DOM (SSR/tests).
// Editor role → semantic token name. The only source of color is the design system.
const ROLE_TOKENS = {
  canvasBg: "surface-sunken",
  border: "border",
  idle: "fg-subtle",
  selected: "danger",
  editing: "success",
  accent: "accent",
  guide: "warning",
  fg: "fg",
  surface: "surface",
} as const;
type Palette = Record<keyof typeof ROLE_TOKENS, string>;

function readPalette(): Palette {
  const cs =
    typeof document === "undefined"
      ? null
      : getComputedStyle(document.documentElement);
  const roles = Object.keys(ROLE_TOKENS) as (keyof typeof ROLE_TOKENS)[];
  return Object.fromEntries(
    roles.map((role) => {
      const token = ROLE_TOKENS[role];
      const resolved = cs?.getPropertyValue(`--color-${token}`).trim();
      return [role, resolved || semanticTokens[token].dark];
    }),
  ) as Palette;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function KonvaCanvasEditor({
  inputs,
  sourceDims,
  gridSize = 10,
  snapToGrid = true,
  showRulers = true,
  showGrid = false,
}: Readonly<Props>) {
  const { canvas, layout, selectedInput } = useLayoutEditorStore(
    useShallow((s) => ({
      canvas: s.canvas,
      layout: s.layout,
      selectedInput: s.selectedInput,
    })),
  );
  const {
    commitSlot,
    commitContent,
    select: onSelect,
  } = useLayoutEditorStore(
    useShallow((s) => ({
      commitSlot: s.commitSlot,
      commitContent: s.commitContent,
      select: s.select,
    })),
  );

  // Container sizing.
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerW, setContainerW] = useState(0);

  // Konva refs.
  const trRef = useRef<Konva.Transformer>(null);
  const regionGroupsRef = useRef<Map<string, Konva.Group>>(new Map());
  const regionHandlesRef = useRef<Map<string, Konva.Rect>>(new Map());
  const activeAnchorRef = useRef<ResizeHandle>(DEFAULT_ANCHOR);

  // Shift = edit-content mode (sync ref + state for re-render).
  const shiftRef = useRef(false);
  const [shiftHeld, setShiftHeld] = useState(false);
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "Shift") {
        shiftRef.current = true;
        setShiftHeld(true);
      }
    };
    const up = (e: KeyboardEvent) => {
      if (e.key === "Shift") {
        shiftRef.current = false;
        setShiftHeld(false);
      }
    };
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
    };
  }, []);

  const panRef = useRef<PanStart | null>(null);
  // Alignment candidates are frozen for the duration of a region drag (the other
  // regions don't move), so compute them once on drag start, not every dragmove.
  const moveCandidatesRef = useRef<{
    vertical: number[];
    horizontal: number[];
  } | null>(null);
  const [liveContent, setLiveContent] = useState<{
    input: string;
    content: ContentTransform;
  } | null>(null);
  const [guides, setGuides] = useState<Guides>({ v: [], h: [] });

  useLayoutEffect(() => {
    if (!containerRef.current) return;
    const el = containerRef.current;
    const ro = new ResizeObserver(() => setContainerW(el.clientWidth));
    ro.observe(el);
    setContainerW(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const stageW = containerW;
  const stageH = containerW / Math.max(1, canvas.w / Math.max(1, canvas.h));
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

  // Resolved design-system colors (read once on mount; re-mount re-reads on theme change).
  const [palette] = useState<Palette>(readPalette);

  // Transformer attaches to the selected region's handle in normal mode; in
  // edit-content (shift) mode the region isn't resized, so it detaches.
  useEffect(() => {
    if (!trRef.current) return;
    const handle =
      selectedInput && !shiftHeld
        ? regionHandlesRef.current.get(selectedInput)
        : undefined;
    trRef.current.nodes(handle ? [handle] : []);
  }, [selectedInput, layout, shiftHeld]);

  // -------------------------------------------------------------------------
  // Drag: region move (normal) or content pan (shift)
  // -------------------------------------------------------------------------

  const otherRegionRefs = useCallback(
    (exclude: string): RegionRef[] =>
      layout
        .filter((s) => s.input !== exclude)
        .map((s) => ({ input: s.input, x: s.x, y: s.y, w: s.w, h: s.h })),
    [layout],
  );

  const handleDragStart = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      const content = backendToContent(slot);
      // Shift edits content (pan) only in cover mode — never silently convert a
      // fit/stretch region to cover; for those, Shift+drag is a normal move.
      if (shiftRef.current && content.fill === "cover") {
        panRef.current = {
          startX: e.target.x(),
          startY: e.target.y(),
          content,
        };
        moveCandidatesRef.current = null;
      } else {
        panRef.current = null;
        moveCandidatesRef.current = collectAlignmentCandidates(
          otherRegionRefs(slot.input),
          slot.input,
          canvas,
        );
      }
    },
    [canvas, otherRegionRefs],
  );

  const doMove = useCallback(
    (slot: LayoutSlot, node: Konva.Node) => {
      // Region is axis-aligned: node x/y ARE region x/y (no coordinate conversion).
      let x = node.x();
      let y = node.y();
      const cand =
        moveCandidatesRef.current ??
        collectAlignmentCandidates(
          otherRegionRefs(slot.input),
          slot.input,
          canvas,
        );
      const snapX = findAlignmentSnap(x, slot.w, cand.vertical);
      const snapY = findAlignmentSnap(y, slot.h, cand.horizontal);
      const gv: number[] = [];
      const gh: number[] = [];
      if (snapX) {
        x = snapX.snappedPos;
        gv.push(snapX.guidePos);
      } else if (snapToGrid && gridSize > 0) x = snapVal(x, gridSize);
      if (snapY) {
        y = snapY.snappedPos;
        gh.push(snapY.guidePos);
      } else if (snapToGrid && gridSize > 0) y = snapVal(y, gridSize);

      const clamped = clampRegionMove(
        { x, y, w: slot.w, h: slot.h },
        canvas,
        false,
        0,
      );
      node.x(clamped.x);
      node.y(clamped.y);
      setGuides({ v: gv, h: gh });
    },
    [canvas, gridSize, otherRegionRefs, snapToGrid],
  );

  const doPan = useCallback(
    (slot: LayoutSlot, node: Konva.Node) => {
      const start = panRef.current!;
      const dxPx = node.x() - start.startX;
      const dyPx = node.y() - start.startY;
      node.x(start.startX);
      node.y(start.startY);
      const src = srcDimsFor(sourceDims, slot);
      // panRef is only armed for cover content (see handleDragStart).
      const { panX, panY } = panContentByPixels({
        region: { w: slot.w, h: slot.h },
        src,
        content: start.content,
        dxPx,
        dyPx,
      });
      setLiveContent({
        input: slot.input,
        content: { ...start.content, panX, panY },
      });
    },
    [sourceDims],
  );

  const handleDragMove = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      if (panRef.current) doPan(slot, e.target);
      else doMove(slot, e.target);
    },
    [doMove, doPan],
  );

  const handleDragEnd = useCallback(
    (slot: LayoutSlot, e: Konva.KonvaEventObject<DragEvent>) => {
      setGuides({ v: [], h: [] });
      if (panRef.current) {
        if (liveContent?.input === slot.input)
          commitContent(slot.input, liveContent.content);
        panRef.current = null;
        setLiveContent(null);
        return;
      }
      const clamped = clampRegionMove(
        { x: e.target.x(), y: e.target.y(), w: slot.w, h: slot.h },
        canvas,
        snapToGrid,
        gridSize,
      );
      e.target.x(clamped.x);
      e.target.y(clamped.y);
      commitSlot(slot.input, { x: clamped.x, y: clamped.y });
    },
    [canvas, commitContent, commitSlot, gridSize, liveContent, snapToGrid],
  );

  // -------------------------------------------------------------------------
  // Resize (region transformer)
  // -------------------------------------------------------------------------

  const handleTransformStart = useCallback(() => {
    activeAnchorRef.current =
      (trRef.current?.getActiveAnchor() as ResizeHandle) ?? "bottom-right";
  }, []);

  const handleTransformEnd = useCallback(
    (slot: LayoutSlot) => {
      const handle = regionHandlesRef.current.get(slot.input);
      const group = regionGroupsRef.current.get(slot.input);
      if (!handle || !group) return;
      const sx = handle.scaleX();
      const sy = handle.scaleY();
      const proposed = {
        x: group.x() + handle.x(),
        y: group.y() + handle.y(),
        w: handle.width() * sx,
        h: handle.height() * sy,
      };
      handle.scaleX(1);
      handle.scaleY(1);
      handle.x(0);
      handle.y(0);

      const anchor = activeAnchorRef.current;
      const next = resizeRegion({
        region: { x: slot.x, y: slot.y, w: slot.w, h: slot.h },
        handle: anchor,
        canvas,
        proposed,
        lockAspect: isCornerAnchor(anchor),
        minSize: MIN_REGION_SIZE,
        doSnap: snapToGrid,
        grid: gridSize,
      });
      group.x(next.x);
      group.y(next.y);
      handle.width(next.w);
      handle.height(next.h);
      commitSlot(slot.input, next);
    },
    [canvas, commitSlot, gridSize, snapToGrid],
  );

  // Live min-size floor + canvas clamp so the box doesn't visibly escape the
  // canvas mid-gesture. Valid axis-aligned math because the region never rotates;
  // handleTransformEnd re-clamps authoritatively via resizeRegion.
  const boundBoxFunc = useCallback(
    (oldBox: Box, newBox: Box) => {
      const minW = MIN_REGION_SIZE * scale;
      const minH = MIN_REGION_SIZE * scale;
      if (Math.abs(newBox.width) < minW || Math.abs(newBox.height) < minH)
        return oldBox;
      const maxW = canvas.w * scale;
      const maxH = canvas.h * scale;
      const c = { ...newBox };
      if (c.x < 0) {
        c.width += c.x;
        c.x = 0;
      }
      if (c.y < 0) {
        c.height += c.y;
        c.y = 0;
      }
      if (c.x + c.width > maxW) c.width = maxW - c.x;
      if (c.y + c.height > maxH) c.height = maxH - c.y;
      if (c.width < minW || c.height < minH) return oldBox;
      return c;
    },
    [canvas.w, canvas.h, scale],
  );

  // -------------------------------------------------------------------------
  // Keyboard nudge (move region)
  // -------------------------------------------------------------------------

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!selectedInput) return;
      const tag = (e.target as HTMLElement | null)?.tagName?.toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select") return;
      const baseStep = e.shiftKey ? 10 : 1;
      const step = snapToGrid && gridSize > 0 ? gridSize : baseStep;
      let dx = 0;
      let dy = 0;
      switch (e.key) {
        case "ArrowLeft":
          dx = -step;
          break;
        case "ArrowRight":
          dx = step;
          break;
        case "ArrowUp":
          dy = -step;
          break;
        case "ArrowDown":
          dy = step;
          break;
        default:
          return;
      }
      e.preventDefault();
      const slot = useLayoutEditorStore
        .getState()
        .layout.find((s) => s.input === selectedInput);
      if (!slot) return;
      const moved = clampRegionMove(
        { x: slot.x + dx, y: slot.y + dy, w: slot.w, h: slot.h },
        canvas,
        false,
        0,
      );
      commitSlot(selectedInput, { x: moved.x, y: moved.y });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [canvas, commitSlot, gridSize, selectedInput, snapToGrid]);

  const handleStageInteract = useCallback(
    (e: Konva.KonvaEventObject<MouseEvent | TouchEvent>) => {
      if (!e.target.getParent()) onSelect(null);
    },
    [onSelect],
  );

  // -------------------------------------------------------------------------
  // Grid lines
  // -------------------------------------------------------------------------

  const gridLines = useMemo(() => {
    if (!showGrid || gridSize <= 0) return [];
    const out: number[][] = [];
    for (let x = gridSize; x < canvas.w; x += gridSize)
      out.push([x, 0, x, canvas.h]);
    for (let y = gridSize; y < canvas.h; y += gridSize)
      out.push([0, y, canvas.w, y]);
    return out;
  }, [canvas.h, canvas.w, gridSize, showGrid]);

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  if (stageW <= 0) return <div ref={containerRef} className="min-w-0" />;

  return (
    <div className="select-none min-w-0 overflow-hidden">
      {showRulers && stageW > 0 && (
        <div className="flex items-start pl-7">
          <CanvasRuler
            size={canvas.w}
            displaySize={stageW}
            orientation="horizontal"
          />
        </div>
      )}
      <div className="flex items-start min-w-0">
        {showRulers && stageH > 0 && (
          <CanvasRuler
            size={canvas.h}
            displaySize={stageH}
            orientation="vertical"
          />
        )}
        <div ref={containerRef} className="flex-1 min-w-0 ml-px">
          <Stage
            width={stageW}
            height={stageH}
            scaleX={scale}
            scaleY={scale}
            onClick={handleStageInteract}
            onTap={handleStageInteract}
          >
            <Layer>
              {/* Background */}
              <Rect
                x={0}
                y={0}
                width={canvas.w}
                height={canvas.h}
                fill={palette.canvasBg}
                stroke={palette.border}
                strokeWidth={2}
                listening={false}
              />

              {/* Grid */}
              {gridLines.map((pts) => (
                <Line
                  key={`gl-${pts[0]}-${pts[1]}`}
                  points={pts}
                  stroke={palette.border}
                  opacity={0.4}
                  strokeWidth={1}
                  listening={false}
                />
              ))}

              {/* Regions */}
              {layout.map((slot) => {
                const isSelected = selectedInput === slot.input;
                const src = srcDimsFor(sourceDims, slot);
                const override =
                  liveContent?.input === slot.input
                    ? liveContent.content
                    : null;
                const content = override ?? backendToContent(slot);
                const fp = computeContentPlacement(
                  { w: slot.w, h: slot.h },
                  src,
                  content,
                );

                const num = slotNumber.get(slot.input) ?? "?";
                const inp = inputByRef.get(slot.input);
                const eff = inp?.effect ? ` · ${inp.effect.type}` : "";
                const rotL = content.rotation ? ` ↻${content.rotation}°` : "";
                const fillL =
                  content.fill === "cover" ? "" : ` [${content.fill}]`;
                const fontSize = Math.max(14, Math.min(slot.w, slot.h) / 10);
                const showGhost =
                  isSelected && shiftHeld && content.fill === "cover";
                let boxStroke = palette.idle;
                if (isSelected)
                  boxStroke = shiftHeld ? palette.editing : palette.selected;

                return (
                  <Group
                    key={slot.input}
                    x={slot.x}
                    y={slot.y}
                    width={slot.w}
                    height={slot.h}
                    draggable
                    ref={(n) => {
                      if (n) regionGroupsRef.current.set(slot.input, n);
                      else regionGroupsRef.current.delete(slot.input);
                    }}
                    onClick={() => onSelect(slot.input)}
                    onTap={() => onSelect(slot.input)}
                    onDragStart={(e) => handleDragStart(slot, e)}
                    onDragMove={(e) => handleDragMove(slot, e)}
                    onDragEnd={(e) => handleDragEnd(slot, e)}
                  >
                    {/* Content, clipped to the region. */}
                    <Group
                      clipX={0}
                      clipY={0}
                      clipWidth={slot.w}
                      clipHeight={slot.h}
                    >
                      <Rect
                        x={fp.x}
                        y={fp.y}
                        width={fp.w}
                        height={fp.h}
                        fill={isSelected ? palette.selected : palette.accent}
                        opacity={0.16}
                        listening={false}
                      />
                      <Text
                        text={`${num}`}
                        x={fp.x + fp.w / 2}
                        y={fp.y + fp.h / 2}
                        width={fp.w}
                        height={fp.h}
                        offsetX={fp.w / 2}
                        offsetY={fp.h / 2}
                        align="center"
                        verticalAlign="middle"
                        rotation={content.rotation}
                        fill={palette.fg}
                        fontSize={fontSize}
                        fontFamily="monospace"
                        listening={false}
                      />
                    </Group>

                    {/* Ghost: full content footprint revealed while editing content. */}
                    {showGhost && (
                      <Rect
                        x={fp.x}
                        y={fp.y}
                        width={fp.w}
                        height={fp.h}
                        stroke={palette.editing}
                        opacity={0.7}
                        strokeWidth={2}
                        dash={[8, 4]}
                        listening={false}
                      />
                    )}

                    {/* Region box + transformer target. */}
                    <Rect
                      ref={(n) => {
                        if (n) regionHandlesRef.current.set(slot.input, n);
                        else regionHandlesRef.current.delete(slot.input);
                      }}
                      width={slot.w}
                      height={slot.h}
                      stroke={boxStroke}
                      strokeWidth={isSelected ? 3 : 2}
                      onTransformEnd={() => handleTransformEnd(slot)}
                    />

                    <Text
                      text={`${num}${eff}${rotL}${fillL}`}
                      x={4}
                      y={4}
                      fill={palette.fg}
                      opacity={0.6}
                      fontSize={Math.max(11, fontSize / 2)}
                      fontFamily="monospace"
                      listening={false}
                    />
                  </Group>
                );
              })}

              {/* Alignment guides */}
              {guides.v.map((x) => (
                <Line
                  key={`gv-${x}`}
                  points={[x, 0, x, canvas.h]}
                  stroke={palette.guide}
                  strokeWidth={1}
                  dash={[6, 4]}
                  listening={false}
                />
              ))}
              {guides.h.map((y) => (
                <Line
                  key={`gh-${y}`}
                  points={[0, y, canvas.w, y]}
                  stroke={palette.guide}
                  strokeWidth={1}
                  dash={[6, 4]}
                  listening={false}
                />
              ))}

              {/* Transformer (region resize): never rotates the box. */}
              <Transformer
                ref={trRef}
                rotateEnabled={false}
                keepRatio
                flipEnabled={false}
                onTransformStart={handleTransformStart}
                boundBoxFunc={boundBoxFunc}
                anchorSize={10}
                anchorCornerRadius={2}
                anchorStroke={palette.accent}
                anchorFill={palette.surface}
                borderStroke={palette.accent}
                borderStrokeWidth={2}
              />
            </Layer>
          </Stage>
        </div>
      </div>
    </div>
  );
}

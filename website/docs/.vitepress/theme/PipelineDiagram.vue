<script setup lang="ts">
/*
 * Hero pipeline topology for the VideoNode home page.
 *
 * Data-driven on purpose: nodes, edges, and lanes are arrays, so the future
 * "AI sensors" lane (auto-crop etc. that attaches to composers) drops in as
 * additional EDGES/box entries rather than a rewrite. Nothing here is drawn
 * that does not ship today.
 *
 * Colour comes entirely from VitePress CSS variables plus three source tints
 * (--vn-t1..3) defined in the scoped style, so light/dark and the brand colour
 * track automatically. Lucide icon path data (MIT) is inlined as raw strings;
 * no icon package is imported.
 */

type Box = { x: number; y: number; w: number; h: number }
type Orient = 'h' | 'v'

const SOURCES = [
  { id: 'hdmi', title: 'HDMI-in', type: 'capture', icon: 'monitor', tint: 1 },
  { id: 'webcam', title: 'Webcam', type: 'USB UVC', icon: 'webcam', tint: 2 },
  { id: 'dongle', title: 'Dongle', type: 'USB capture', icon: 'usb', tint: 3 },
] as const

const STREAMS = [
  { id: 's-a', title: 'stream-A', codec: 'H.264' },
  { id: 's-b', title: 'stream-B', codec: 'H.264' },
  { id: 's-c', title: 'stream-C', codec: 'HEVC' },
] as const

// from -> to, with the four flow stories this diagram has to tell:
//  in     : a typed source feeds the composer (carries the source tint)
//  direct : a source bypasses the composer and feeds a stream directly
//  out    : the composer fans out to multiple encoder streams
const EDGES = [
  { from: 'hdmi', to: 'c1', kind: 'in', tint: 1 },
  { from: 'webcam', to: 'c1', kind: 'in', tint: 2 },
  { from: 'dongle', to: 'c1', kind: 'in', tint: 3 },
  { from: 'hdmi', to: 's-a', kind: 'direct', tint: 1 },
  { from: 'c1', to: 's-b', kind: 'out', tint: 0 },
  { from: 'c1', to: 's-c', kind: 'out', tint: 0 },
] as const

const ICONS: Record<string, string[]> = {
  monitor: [
    'M4 3h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z',
    'M8 21h8',
    'M12 17v4',
  ],
  webcam: [
    'M12 18a8 8 0 1 0 0-16 8 8 0 0 0 0 16z',
    'M12 13a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
    'M7 22h10',
    'M12 18v4',
  ],
  usb: [
    'M10 8a1 1 0 1 0 0-2 1 1 0 0 0 0 2z',
    'M4 21a1 1 0 1 0 0-2 1 1 0 0 0 0 2z',
    'M4.7 19.3 19 5',
    'M21 3l-3 1 2 2z',
    'M9.26 7.68 5 12l2 5',
    'M10 14l5 2 3.5-3.5',
    'M18 12l1-1 1 1-1 1z',
  ],
  radio: [
    'M4.9 16.1C1 12.2 1 5.8 4.9 1.9',
    'M7.8 4.7a6.14 6.14 0 0 0-.8 7.5',
    'M12 11a2 2 0 1 0 0-4 2 2 0 0 0 0 4z',
    'M16.2 4.8c2 2 2.26 5.11.8 7.47',
    'M19.1 1.9a9.96 9.96 0 0 1 0 14.1',
    'M9.5 18h5',
    'M8 22l4-11 4 11',
  ],
  layers: [
    'M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z',
    'M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12',
    'M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17',
  ],
}

type Pt = { x: number; y: number }

// Flow-dot travel speed in viewBox units per second. Each dot's duration is
// its edge length / this, so dots move at the same pace on every edge.
const FLOW_SPEED = 55

function cubicLen(p0: Pt, p1: Pt, p2: Pt, p3: Pt) {
  let len = 0
  let prev = p0
  for (let i = 1; i <= 24; i++) {
    const t = i / 24
    const mt = 1 - t
    const x = mt * mt * mt * p0.x + 3 * mt * mt * t * p1.x + 3 * mt * t * t * p2.x + t * t * t * p3.x
    const y = mt * mt * mt * p0.y + 3 * mt * mt * t * p1.y + 3 * mt * t * t * p2.y + t * t * t * p3.y
    len += Math.hypot(x - prev.x, y - prev.y)
    prev = { x, y }
  }
  return len
}

function edgeGeom(a: Pt, b: Pt, orient: Orient, kind: string): { d: string; len: number } {
  let p1: Pt
  let p2: Pt
  if (orient === 'h') {
    if (kind === 'direct') {
      p1 = { x: a.x + 96, y: a.y + 86 }
      p2 = { x: b.x - 96, y: b.y + 14 }
    } else {
      const k = Math.max(44, Math.abs(b.x - a.x) * 0.45)
      p1 = { x: a.x + k, y: a.y }
      p2 = { x: b.x - k, y: b.y }
    }
  } else if (kind === 'direct') {
    return { d: `M${a.x},${a.y} L${b.x},${b.y}`, len: Math.hypot(b.x - a.x, b.y - a.y) }
  } else {
    // clamp so short, near-vertical edges (composer -> stream) don't overshoot
    const k = Math.min(40, Math.abs(b.y - a.y) * 0.5)
    p1 = { x: a.x, y: a.y + k }
    p2 = { x: b.x, y: b.y - k }
  }
  const d = `M${a.x},${a.y} C${p1.x},${p1.y} ${p2.x},${p2.y} ${b.x},${b.y}`
  return { d, len: cubicLen(a, p1, p2, b) }
}

function build(orient: Orient) {
  const box: Record<string, Box> = {}
  let viewBox: string
  if (orient === 'h') {
    box.hdmi = { x: 20, y: 34, w: 150, h: 70 }
    box.webcam = { x: 20, y: 150, w: 150, h: 70 }
    box.dongle = { x: 20, y: 266, w: 150, h: 70 }
    box.c1 = { x: 344, y: 92, w: 178, h: 188 }
    box['s-a'] = { x: 636, y: 40, w: 150, h: 64 }
    box['s-b'] = { x: 636, y: 156, w: 150, h: 64 }
    box['s-c'] = { x: 636, y: 272, w: 150, h: 64 }
    viewBox = '0 0 1040 372'
  } else {
    // webcam sits on the right edge, directly above its direct stream (s-c),
    // so the bypass edge is a straight vertical line clear of the composer.
    box.hdmi = { x: 14, y: 18, w: 108, h: 92 }
    box.dongle = { x: 132, y: 18, w: 108, h: 92 }
    box.webcam = { x: 250, y: 18, w: 108, h: 92 }
    box.c1 = { x: 86, y: 172, w: 200, h: 172 }
    box['s-a'] = { x: 14, y: 410, w: 108, h: 78 }
    box['s-b'] = { x: 132, y: 410, w: 108, h: 78 }
    box['s-c'] = { x: 250, y: 410, w: 108, h: 78 }
    viewBox = '0 0 372 528'
  }
  const out = (id: string) =>
    orient === 'h'
      ? { x: box[id].x + box[id].w, y: box[id].y + box[id].h / 2 }
      : { x: box[id].x + box[id].w / 2, y: box[id].y + box[id].h }
  const inp = (id: string) =>
    orient === 'h'
      ? { x: box[id].x, y: box[id].y + box[id].h / 2 }
      : { x: box[id].x + box[id].w / 2, y: box[id].y }
  const edges = EDGES.map((e, i) => {
    const g = edgeGeom(out(e.from), inp(e.to), orient, e.kind)
    return { ...e, i, d: g.d, len: g.len }
  })
  return { key: orient, orient, viewBox, box, edges }
}

// Rendered beside the hero (home-hero-image slot), so only the portrait
// orientation is used; build('h') is kept for a potential full-width placement.
const LAYOUTS = [build('v')]

const ALT =
  'VideoNode pipeline. Three video sources, an HDMI-in capture, a USB webcam and a USB capture dongle, fan into a composer that lays them out on one canvas. The composer feeds two encoder streams, one H.264 and one HEVC, and the HDMI capture also feeds a stream directly. Every stream publishes over RTSP, SRT and WebRTC.'

function iconTransform(cx: number, cy: number, size = 22) {
  const s = size / 24
  return `translate(${cx - size / 2}, ${cy - size / 2}) scale(${s})`
}

function tintVar(t: number) {
  return t ? `var(--vn-t${t})` : 'var(--vp-c-brand-1)'
}

// Composer canvas, base arrangement A: a big tile (hdmi) left, two stacked
// tiles (webcam, dongle) right. Final pixel coords (no inset applied in the
// template) so the vn-reflow keyframes can target the same geometry exactly.
// Box is 200x172; 16px top pad, 112px canvas, leaving a 44px label strip.
function canvasTiles(b: Box) {
  const fx = b.x + 16
  const fy = b.y + 16
  const fw = b.w - 32
  const fh = 112
  // Inset the tiles 3px inside the framed canvas so the semi-transparent tints
  // never overlap the frame's border stroke (which reads as a muddy edge line).
  const ix = fx + 3
  const iy = fy + 3
  const iw = fw - 6
  const ih = fh - 6
  const gap = 6
  const bigW = Math.round((iw - gap) * 0.58)
  const smallW = iw - gap - bigW
  const smallH = (ih - gap) / 2
  return {
    frame: { x: fx, y: fy, w: fw, h: fh },
    tiles: [
      { x: ix, y: iy, w: bigW, h: ih, tint: 1 },
      { x: ix + bigW + gap, y: iy, w: smallW, h: smallH, tint: 2 },
      { x: ix + bigW + gap, y: iy + smallH + gap, w: smallW, h: smallH, tint: 3 },
    ],
  }
}
</script>

<template>
  <figure class="vn-diagram" role="img" :aria-label="ALT">
    <svg
      v-for="L in LAYOUTS"
      :key="L.key"
      :class="['vn-svg', L.orient === 'h' ? 'vn-wide' : 'vn-tall']"
      :viewBox="L.viewBox"
      aria-hidden="true"
      preserveAspectRatio="xMidYMid meet"
    >
      <defs>
        <marker
          :id="`vn-arrow-${L.key}`"
          viewBox="0 0 10 10"
          refX="8"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M1 1 L9 5 L1 9" fill="none" stroke="context-stroke" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </marker>
      </defs>

      <!-- edges -->
      <g class="vn-edges">
        <path
          v-for="e in L.edges"
          :key="e.i"
          class="vn-edge"
          :class="`vn-edge-${e.kind}`"
          :d="e.d"
          pathLength="1"
          fill="none"
          :stroke="tintVar(e.tint)"
          :marker-end="`url(#vn-arrow-${L.key})`"
          :style="{ animationDelay: `${0.15 + e.i * 0.08}s` }"
        />
        <circle
          v-for="e in L.edges"
          :key="`f-${e.i}`"
          class="vn-flow"
          r="3"
          :fill="tintVar(e.tint)"
          :style="{
            offsetPath: `path('${e.d}')`,
            animationDuration: `${(e.len / FLOW_SPEED).toFixed(2)}s`,
            animationDelay: `${e.i * 0.4}s`,
          }"
        />
      </g>

      <!-- sources -->
      <g v-for="s in SOURCES" :key="s.id" class="vn-node">
        <rect class="vn-card" :x="L.box[s.id].x" :y="L.box[s.id].y" :width="L.box[s.id].w" :height="L.box[s.id].h" rx="12" />
        <template v-if="L.orient === 'h'">
          <rect class="vn-accent" :x="L.box[s.id].x" :y="L.box[s.id].y + 12" width="4" :height="L.box[s.id].h - 24" rx="2" :fill="tintVar(s.tint)" />
          <g class="vn-icon" :transform="iconTransform(L.box[s.id].x + 34, L.box[s.id].y + L.box[s.id].h / 2, 24)" :style="{ color: tintVar(s.tint) }">
            <path v-for="(d, i) in ICONS[s.icon]" :key="i" :d="d" />
          </g>
          <text class="vn-title" :x="L.box[s.id].x + 62" :y="L.box[s.id].y + 32">{{ s.title }}</text>
          <text class="vn-sub" :x="L.box[s.id].x + 62" :y="L.box[s.id].y + 50">{{ s.type }}</text>
        </template>
        <template v-else>
          <rect class="vn-accent" :x="L.box[s.id].x + 12" :y="L.box[s.id].y" :width="L.box[s.id].w - 24" height="4" rx="2" :fill="tintVar(s.tint)" />
          <g class="vn-icon" :transform="iconTransform(L.box[s.id].x + L.box[s.id].w / 2, L.box[s.id].y + 34, 24)" :style="{ color: tintVar(s.tint) }">
            <path v-for="(d, i) in ICONS[s.icon]" :key="i" :d="d" />
          </g>
          <text class="vn-title vn-mid" :x="L.box[s.id].x + L.box[s.id].w / 2" :y="L.box[s.id].y + 66">{{ s.title }}</text>
          <text class="vn-sub vn-mid" :x="L.box[s.id].x + L.box[s.id].w / 2" :y="L.box[s.id].y + 82">{{ s.type }}</text>
        </template>
      </g>

      <!-- composer -->
      <g class="vn-node vn-composer">
        <rect class="vn-card vn-card-strong" :x="L.box.c1.x" :y="L.box.c1.y" :width="L.box.c1.w" :height="L.box.c1.h" rx="14" />
        <g>
          <rect
            class="vn-canvas"
            :x="canvasTiles(L.box.c1).frame.x"
            :y="canvasTiles(L.box.c1).frame.y"
            :width="canvasTiles(L.box.c1).frame.w"
            :height="canvasTiles(L.box.c1).frame.h"
            rx="0"
          />
          <rect
            v-for="(t, i) in canvasTiles(L.box.c1).tiles"
            :key="i"
            :class="['vn-tile', `vn-tile-${i}`]"
            :x="t.x"
            :y="t.y"
            :width="t.w"
            :height="t.h"
            rx="0"
            :fill="tintVar(t.tint)"
          />
        </g>
        <g class="vn-icon vn-icon-muted" :transform="iconTransform(L.box.c1.x + 24, L.box.c1.y + L.box.c1.h - 22, 15)">
          <path v-for="(d, i) in ICONS.layers" :key="i" :d="d" />
        </g>
        <text class="vn-title" :x="L.box.c1.x + 42" :y="L.box.c1.y + L.box.c1.h - 24">Composer</text>
        <text class="vn-sub" :x="L.box.c1.x + 42" :y="L.box.c1.y + L.box.c1.h - 10">GPU canvas</text>
      </g>

      <!-- streams + outputs -->
      <g v-for="st in STREAMS" :key="st.id" class="vn-node">
        <rect class="vn-card" :x="L.box[st.id].x" :y="L.box[st.id].y" :width="L.box[st.id].w" :height="L.box[st.id].h" rx="12" />
        <template v-if="L.orient === 'h'">
          <g class="vn-icon vn-icon-brand" :transform="iconTransform(L.box[st.id].x + 28, L.box[st.id].y + L.box[st.id].h / 2, 22)">
            <path v-for="(d, i) in ICONS.radio" :key="i" :d="d" />
          </g>
          <text class="vn-title" :x="L.box[st.id].x + 52" :y="L.box[st.id].y + L.box[st.id].h / 2 + 5">{{ st.title }}</text>
          <line class="vn-tap" :x1="L.box[st.id].x + L.box[st.id].w" :y1="L.box[st.id].y + L.box[st.id].h / 2" :x2="L.box[st.id].x + L.box[st.id].w + 16" :y2="L.box[st.id].y + L.box[st.id].h / 2" />
          <rect class="vn-pill" :x="L.box[st.id].x + L.box[st.id].w + 16" :y="L.box[st.id].y + L.box[st.id].h / 2 - 15" width="210" height="30" rx="15" />
          <text class="vn-proto vn-mid" :x="L.box[st.id].x + L.box[st.id].w + 16 + 105" :y="L.box[st.id].y + L.box[st.id].h / 2 + 4">RTSP · SRT · WebRTC</text>
        </template>
        <template v-else>
          <g class="vn-icon vn-icon-brand" :transform="iconTransform(L.box[st.id].x + L.box[st.id].w / 2, L.box[st.id].y + 20, 18)">
            <path v-for="(d, i) in ICONS.radio" :key="i" :d="d" />
          </g>
          <text class="vn-title vn-mid" :x="L.box[st.id].x + L.box[st.id].w / 2" :y="L.box[st.id].y + 44">{{ st.title }}</text>
          <text class="vn-codec vn-mid" :x="L.box[st.id].x + L.box[st.id].w / 2" :y="L.box[st.id].y + 60">{{ st.codec }}</text>
          <text class="vn-proto vn-mid" :x="L.box[st.id].x + L.box[st.id].w / 2" :y="L.box[st.id].y + L.box[st.id].h + 16">RTSP·SRT·WebRTC</text>
        </template>
      </g>
    </svg>
  </figure>
</template>

<style scoped>
.vn-diagram {
  margin: 0 auto;
  max-width: 100%;
  padding: 0;

  --vn-t1: #1f9bb0;
  --vn-t2: #c77d33;
  --vn-t3: #6f60e6;
}
:global(.dark) .vn-diagram {
  --vn-t1: #44cfdd;
  --vn-t2: #f0a85e;
  --vn-t3: #9a8cff;
}

.vn-svg {
  display: block;
  margin: 0 auto;
  height: clamp(360px, 48vh, 520px);
  width: auto;
  max-width: 100%;
  aspect-ratio: 372 / 528;
  overflow: visible;
}

/* cards */
.vn-card {
  fill: var(--vp-c-bg-soft);
  stroke: var(--vp-c-border);
  stroke-width: 1;
}
.vn-card-strong {
  fill: var(--vp-c-bg-elv);
}
.vn-canvas {
  fill: var(--vp-c-bg);
  stroke: var(--vp-c-border);
  stroke-width: 1;
}
.vn-tile {
  opacity: 0.85;
}

/* text */
.vn-title {
  fill: var(--vp-c-text-1);
  font-family: var(--vp-font-family-base);
  font-size: 14px;
  font-weight: 600;
}
.vn-sub,
.vn-proto {
  fill: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
  font-size: 10.5px;
  letter-spacing: 0.02em;
}
.vn-proto {
  fill: var(--vp-c-brand-1);
  font-size: 11px;
}
.vn-codec {
  fill: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 10.5px;
  letter-spacing: 0.03em;
}
.vn-mid {
  text-anchor: middle;
}

/* icons */
.vn-icon path {
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.vn-icon {
  color: var(--vp-c-text-1);
}
.vn-icon-brand {
  color: var(--vp-c-brand-1);
}
.vn-icon-muted {
  color: var(--vp-c-text-3);
}

/* edges */
.vn-edge {
  stroke-width: 1.6;
  opacity: 0.8;
  stroke-linecap: round;
  stroke-dasharray: 1;
  stroke-dashoffset: 0;
}
.vn-tap {
  stroke: var(--vp-c-border);
  stroke-width: 1.4;
}
.vn-pill {
  fill: var(--vp-c-bg-soft);
  stroke: var(--vp-c-border);
  stroke-width: 1;
}

.vn-flow {
  opacity: 0;
}

@media (prefers-reduced-motion: no-preference) {
  .vn-diagram {
    opacity: 0;
    transform: translateY(10px);
    animation: vn-fade 0.8s ease 0.05s forwards;
  }
  .vn-edge {
    stroke-dashoffset: 1;
    animation: vn-draw 0.7s ease forwards;
  }
  .vn-flow {
    animation: vn-travel 2.6s linear infinite;
  }
  /*
   * Live recomposition: all three tiles cycle through three distinct PiP
   * arrangements (A -> B -> C -> A), each held ~10s with a ~3s transition, so
   * the composer reads as occasionally re-laying-out rather than drifting one
   * way. Coords are final canvas pixels matching canvasTiles() arrangement A.
   * 39s loop: hold 10s (0-25.6%), move 3s, hold 10s (33.3-59%), move 3s, etc.
   */
  .vn-tile-0 {
    animation: vn-reflow-0 39s ease-in-out infinite;
  }
  .vn-tile-1 {
    animation: vn-reflow-1 39s ease-in-out infinite;
  }
  .vn-tile-2 {
    animation: vn-reflow-2 39s ease-in-out infinite;
  }
}

@keyframes vn-fade {
  to {
    opacity: 1;
    transform: none;
  }
}
@keyframes vn-draw {
  to {
    stroke-dashoffset: 0;
  }
}
/*
 * The three tiles are a treemap: t0 is the full-height left column; t1/t2 share
 * the right column (top/bottom). Only the vertical split (t0 width) and the
 * horizontal split (t1/t2 heights) move, and the 6px gaps are identical at
 * every keyframe, so linear interpolation keeps a perfect tiling: the tiles
 * resize but can never overlap. Canvas is 102..270 x 188..300.
 *   A: balanced (hdmi main)   B: dongle main   C: webcam main
 * Coords match canvasTiles()'s 3px-inset tile region: x 105..267, y 191..297.
 */
@keyframes vn-reflow-0 {
  0%, 25.6% { x: 105px; y: 191px; width: 90px; height: 106px; }
  33.3%, 59% { x: 105px; y: 191px; width: 44px; height: 106px; }
  66.7%, 92.3% { x: 105px; y: 191px; width: 96px; height: 106px; }
  100% { x: 105px; y: 191px; width: 90px; height: 106px; }
}
@keyframes vn-reflow-1 {
  0%, 25.6% { x: 201px; y: 191px; width: 66px; height: 50px; }
  33.3%, 59% { x: 155px; y: 191px; width: 112px; height: 22px; }
  66.7%, 92.3% { x: 207px; y: 191px; width: 60px; height: 78px; }
  100% { x: 201px; y: 191px; width: 66px; height: 50px; }
}
@keyframes vn-reflow-2 {
  0%, 25.6% { x: 201px; y: 247px; width: 66px; height: 50px; }
  33.3%, 59% { x: 155px; y: 219px; width: 112px; height: 78px; }
  66.7%, 92.3% { x: 207px; y: 275px; width: 60px; height: 22px; }
  100% { x: 201px; y: 247px; width: 66px; height: 50px; }
}
@keyframes vn-travel {
  0% {
    offset-distance: 0%;
    opacity: 0;
  }
  12% {
    opacity: 1;
  }
  85% {
    opacity: 1;
  }
  100% {
    offset-distance: 100%;
    opacity: 0;
  }
}
</style>

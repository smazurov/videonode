// Local type stubs for the new Composer shape (plan U9).
//
// These mirror the canonical Go types in `internal/streams/pipeline/composer.go`
// from the plan. Once backend units B1/B6 land and `pnpm gen:api` is rerun
// (unit U1), these will be replaced by `components['schemas']['Composer']`
// etc. from `api.generated.ts`. Keep field names identical to the eventual
// OpenAPI snake_case shape so the swap is mechanical.

export interface CanvasDims {
  w: number;
  h: number;
  fps?: number;
  // Hex color the sources composite on top of ("#RRGGBB" / "#RRGGBBAA").
  // Empty/undefined renders as opaque black.
  background?: string;
}

// Daemon-side default canvas background when unset. Mirrors the opaque
// black the composer clears to.
export const DEFAULT_CANVAS_BACKGROUND = '#000000';

// Validate a canvas background hex string. Empty/undefined is valid (the
// composer falls back to opaque black). Mirrors the backend pattern in
// api/models/composers.go (#RRGGBB or #RRGGBBAA, '#' optional).
export function validateBackground(background: string | undefined): string | undefined {
  if (!background) return undefined;
  return /^#?([\dA-Fa-f]{6}|[\dA-Fa-f]{8})$/.test(background)
    ? undefined
    : 'Must be #RRGGBB or #RRGGBBAA hex';
}

// Daemon-side default canvas frame rate when fps is unset (0 / undefined).
// Mirrors pipeline.DefaultCanvasFPS in the Go daemon.
export const DEFAULT_CANVAS_FPS = 60;

export type EffectType = 'perspective';

export interface Effect {
  type: EffectType;
  corners?: [
    [number, number],
    [number, number],
    [number, number],
    [number, number],
  ];
}

export interface ComposerInput {
  ref: string; // "source:<id>"
  effect?: Effect | null;
}

export type AspectRatioMode = 'stretch' | 'fit' | 'crop';

export interface CropConfig {
  x: number;
  y: number;
  scale: number;
}

export interface LayoutSlot {
  input: string; // matches ComposerInput.ref
  x: number;
  y: number;
  w: number;
  h: number;
  rotation?: number; // 0, 90, 180, 270 clockwise degrees
  aspect_ratio_mode?: AspectRatioMode;
  crop?: CropConfig;
}

export interface Composer {
  id: string;
  canvas: CanvasDims;
  inputs: ComposerInput[];
  layout: LayoutSlot[];
  created_at?: string;
  updated_at?: string;
}

// PATCH /api/composers/{id}/layout body shape.
export interface ComposerLayoutPatch {
  layout: LayoutSlot[];
}

export interface ComposerData extends Omit<Composer, 'id'> {
  composer_id: string;
  status?: string;
  downstream_stream_ids?: string[];
}

export function formatCanvasDims(c: CanvasDims): string {
  return `${c.w}x${c.h}`;
}

export function canvasFpsOrDefault(c: CanvasDims): number {
  return c.fps && c.fps > 0 ? c.fps : DEFAULT_CANVAS_FPS;
}

// Per-field validation errors for CanvasDims (undefined = field is valid).
export type CanvasValidationErrors = Record<'w' | 'h' | 'fps', string | undefined>;

// Validate canvas dims + fps using the same rules as the backend
// (services/composer_service.go validateComposerCreate). Returns
// per-field error messages keyed by `w` / `h` / `fps`; an empty object
// means the input is valid.
export function validateCanvas(dims: CanvasDims): CanvasValidationErrors {
  const errors: CanvasValidationErrors = { w: undefined, h: undefined, fps: undefined };
  if (!Number.isFinite(dims.w) || dims.w < 16 || dims.w > 7680) errors.w = 'Width must be 16-7680';
  else if (dims.w % 2 !== 0) errors.w = 'Width must be even';
  if (!Number.isFinite(dims.h) || dims.h < 16 || dims.h > 4320) errors.h = 'Height must be 16-4320';
  else if (dims.h % 2 !== 0) errors.h = 'Height must be even';
  const fps = dims.fps ?? DEFAULT_CANVAS_FPS;
  if (!Number.isFinite(fps) || fps < 1 || fps > 240) errors.fps = 'Frame rate must be 1-240';
  return errors;
}

export function hasCanvasErrors(errors: CanvasValidationErrors): boolean {
  return Boolean(errors.w || errors.h || errors.fps);
}

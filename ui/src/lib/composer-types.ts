// Stub composer types — mirror the canonical Go shapes from
// internal/streams/pipeline/composer.go. Generated types in
// api.generated.ts will replace these once the backend OpenAPI regen
// from B6 lands; consumers import from this module so the swap is
// mechanical.

export interface CanvasDims {
  w: number;
  h: number;
}

export interface ComposerEffectData {
  type: string;
  corners?: [number, number][];
}

export interface ComposerInputData {
  ref: string;
  effect?: ComposerEffectData;
}

export interface ComposerLayoutSlot {
  input: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export type ComposerStatus = "warm" | "cold" | "error" | "idle";

export interface ComposerData {
  composer_id: string;
  canvas: CanvasDims;
  inputs: ComposerInputData[];
  layout: ComposerLayoutSlot[];
  status?: ComposerStatus;
  downstream_stream_ids?: string[];
  created_at?: string;
  updated_at?: string;
}

export interface ComposerListData {
  composers: ComposerData[];
}

export function formatCanvasDims(dims: CanvasDims): string {
  return `${dims.w}x${dims.h}`;
}

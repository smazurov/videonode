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
}

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

export interface LayoutSlot {
  input: string; // matches ComposerInput.ref
  x: number;
  y: number;
  w: number;
  h: number;
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

import type { StateCreator } from 'zustand';

import type { CanvasDims, LayoutSlot } from '../../../lib/composer-types';
import type { LayoutEditorStore } from '../../useLayoutEditorStore';

export interface LayoutEditorSlice {
  canvas: CanvasDims;
  layout: LayoutSlot[];
  selectedInput: string | null;

  setCanvas: (canvas: CanvasDims) => void;
  commitSlot: (input: string, updates: Partial<LayoutSlot>) => void;
  select: (input: string | null) => void;
}

export const createLayoutEditorSlice: StateCreator<
  LayoutEditorStore, [], [], LayoutEditorSlice
> = (set, get) => ({
  canvas: { w: 1920, h: 1080 },
  layout: [],
  selectedInput: null,

  setCanvas: (canvas) => set({ canvas }),

  commitSlot: (input, updates) => {
    const { layout, pushHistory } = get();
    const next = layout.map((s) => s.input === input ? { ...s, ...updates } : s);
    pushHistory(layout);
    set({ layout: next });
  },

  select: (input) => set({ selectedInput: input }),
});

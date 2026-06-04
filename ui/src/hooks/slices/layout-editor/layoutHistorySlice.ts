import type { StateCreator } from 'zustand';

import type { LayoutSlot } from '../../../lib/composer-types';
import type { LayoutEditorStore } from '../../useLayoutEditorStore';

const MAX_HISTORY = 50;

export interface LayoutHistorySlice {
  past: LayoutSlot[][];
  future: LayoutSlot[][];

  pushHistory: (snapshot: LayoutSlot[]) => void;
  setLayout: (next: LayoutSlot[]) => void;
  undo: () => void;
  redo: () => void;
  resetHistory: (layout: LayoutSlot[]) => void;
}

export const createLayoutHistorySlice: StateCreator<
  LayoutEditorStore, [], [], LayoutHistorySlice
> = (set, get) => ({
  past: [],
  future: [],

  pushHistory: (snapshot) => {
    const newPast = [...get().past, snapshot];
    if (newPast.length > MAX_HISTORY) newPast.shift();
    set({ past: newPast, future: [] });
  },

  setLayout: (next) => {
    const { layout } = get();
    get().pushHistory(layout);
    set({ layout: next });
  },

  undo: () => {
    const { past, layout, future } = get();
    if (past.length === 0) return;
    const prev = past[past.length - 1]!;
    set({
      past: past.slice(0, -1),
      layout: prev,
      future: [layout, ...future],
    });
  },

  redo: () => {
    const { past, layout, future } = get();
    if (future.length === 0) return;
    const next = future[0]!;
    set({
      past: [...past, layout],
      layout: next,
      future: future.slice(1),
    });
  },

  resetHistory: (layout) => set({ layout, past: [], future: [] }),
});

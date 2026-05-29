import { create } from "zustand";
import { subscribeWithSelector } from "zustand/middleware";

import {
  createLayoutEditorSlice,
  type LayoutEditorSlice,
} from "./slices/layout-editor/layoutEditorSlice";
import {
  createLayoutHistorySlice,
  type LayoutHistorySlice,
} from "./slices/layout-editor/layoutHistorySlice";

export interface LayoutEditorStore
  extends LayoutEditorSlice, LayoutHistorySlice {}

export const useLayoutEditorStore = create<LayoutEditorStore>()(
  subscribeWithSelector((set, get, store) => ({
    ...createLayoutEditorSlice(set, get, store),
    ...createLayoutHistorySlice(set, get, store),
  })),
);

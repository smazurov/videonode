import { useEffect, useMemo } from 'react';

import { buildSourceDims, layoutFootprints } from '../components/composers/canvas-mask';
import type { Rect, Size } from '../components/composers/region-content';
import type { Composer } from './slices/types';
import { useComposerStore } from './useComposerStore';
import { useSourceStore } from './useSourceStore';
import { useStreamStore } from './useStreamStore';

export interface ComposerMask {
  readonly composerId: string;
  readonly canvas: Size;
  readonly rects: readonly Rect[];
  /** Fit-mode inputs whose source reports no frame size, so their letterbox is unknown. */
  readonly unsizedInputs: readonly string[];
}

/** Video-coverage rects for a composer's canvas. Null until the composer is loaded. */
export function useComposerMask(composer: Composer | undefined): ComposerMask | null {
  const sourcesById = useSourceStore((s) => s.sourcesById);

  return useMemo(() => {
    if (!composer) return null;
    const layout = composer.layout ?? [];
    const canvas = { w: composer.canvas.w, h: composer.canvas.h };
    const sourceDims = buildSourceDims(composer.inputs ?? [], sourcesById);
    return {
      composerId: composer.id,
      canvas,
      rects: layoutFootprints(layout, canvas, sourceDims),
      unsizedInputs: layout
        .filter((slot) => slot.aspect_ratio_mode === 'fit' && !sourceDims.has(slot.input))
        .map((slot) => slot.input),
    };
  }, [composer, sourcesById]);
}

/**
 * Same, for the composer a stream reads from. Null when the stream reads a
 * source directly. Loads the composer and source stores on demand, since the
 * stream routes don't otherwise need them.
 */
export function useUpstreamComposerMask(streamId: string): ComposerMask | null {
  const upstream = useStreamStore((s) => s.streamsById[streamId]?.upstream);
  const composerId = upstream?.startsWith('composer:') ? upstream.slice('composer:'.length) : '';

  const composer = useComposerStore((s) => (composerId ? s.composersById[composerId] : undefined));
  const composersLastUpdated = useComposerStore((s) => s.lastUpdated);
  const fetchComposers = useComposerStore((s) => s.fetchComposers);
  const sourcesLastUpdated = useSourceStore((s) => s.lastUpdated);
  const fetchSources = useSourceStore((s) => s.fetchSources);

  useEffect(() => {
    if (composerId && composersLastUpdated === null) void fetchComposers();
  }, [composerId, composersLastUpdated, fetchComposers]);

  useEffect(() => {
    if (composerId && sourcesLastUpdated === null) void fetchSources();
  }, [composerId, sourcesLastUpdated, fetchSources]);

  return useComposerMask(composer);
}

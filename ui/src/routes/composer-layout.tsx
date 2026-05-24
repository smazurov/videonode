import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';

import { useAuthStore } from '../hooks/useAuthStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { Checkbox } from '../components/Checkbox';
import { InputField } from '../components/InputField';
import { CanvasEditor } from '../components/composers/CanvasEditor';
import { LayoutSlotInspector } from '../components/composers/LayoutSlotInspector';
import { API_BASE_URL } from '../lib/api';
import { getAuthCredentials } from '../lib/auth';
import type { Composer, LayoutSlot } from '../lib/composer-types';

const SAVE_DEBOUNCE_MS = 250;

interface FetchState {
  loading: boolean;
  error: string | null;
  composer: Composer | null;
}

async function fetchComposer(id: string): Promise<Composer> {
  const credentials = getAuthCredentials();
  if (!credentials) throw new Error('Not authenticated');
  const res = await fetch(`${API_BASE_URL}/api/composers/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Basic ${credentials}` },
  });
  if (!res.ok) throw new Error(`Failed to fetch composer: ${res.status} ${res.statusText}`);
  return (await res.json()) as Composer;
}

async function patchLayout(id: string, layout: LayoutSlot[]): Promise<void> {
  const credentials = getAuthCredentials();
  if (!credentials) throw new Error('Not authenticated');
  const res = await fetch(`${API_BASE_URL}/api/composers/${encodeURIComponent(id)}/layout`, {
    method: 'PATCH',
    headers: {
      Authorization: `Basic ${credentials}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ layout }),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(`Save failed: ${res.status} ${text}`);
  }
}

export default function ComposerLayoutRoute() {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  const [state, setState] = useState<FetchState>({
    loading: true,
    error: null,
    composer: null,
  });
  const [selectedInput, setSelectedInput] = useState<string | null>(null);
  const [snapToGrid, setSnapToGrid] = useState(true);
  const [gridSize, setGridSize] = useState(10);
  const [showRulers, setShowRulers] = useState(true);
  const [saving, setSaving] = useState(false);

  // Last persisted layout — used to roll back on save failure (optimistic UI).
  const persistedRef = useRef<LayoutSlot[] | null>(null);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingRef = useRef<LayoutSlot[] | null>(null);

  const onFetchSuccess = useCallback((c: Composer) => {
    persistedRef.current = [...c.layout];
    setState({ loading: false, error: null, composer: c });
    if (c.layout.length > 0) setSelectedInput(c.layout[0]?.input ?? null);
  }, []);

  const onFetchError = useCallback((error: unknown) => {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    setState({ loading: false, error: msg, composer: null });
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetchComposer(id)
      .then((c) => {
        if (cancelled) return;
        onFetchSuccess(c);
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        onFetchError(error);
      });
    return () => {
      cancelled = true;
    };
  }, [id, onFetchSuccess, onFetchError]);

  const rollbackToPersisted = useCallback(() => {
    const rollback = persistedRef.current;
    if (!rollback) return;
    setState((prev) =>
      prev.composer
        ? { ...prev, composer: { ...prev.composer, layout: [...rollback] } }
        : prev,
    );
  }, []);

  const handleSaveError = useCallback(
    (error: unknown) => {
      const msg = error instanceof Error ? error.message : 'Save failed';
      toast.error(msg);
      rollbackToPersisted();
    },
    [rollbackToPersisted],
  );

  const runSave = useCallback(() => {
    const toSave = pendingRef.current;
    if (!toSave) return;
    setSaving(true);
    patchLayout(id, toSave)
      .then(() => {
        persistedRef.current = toSave;
      })
      .catch(handleSaveError)
      .finally(() => {
        setSaving(false);
      });
  }, [handleSaveError, id]);

  const scheduleSave = useCallback(
    (layout: LayoutSlot[]) => {
      pendingRef.current = layout;
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      saveTimerRef.current = setTimeout(runSave, SAVE_DEBOUNCE_MS);
    },
    [runSave],
  );

  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, []);

  const handleLayoutChange = useCallback(
    (next: LayoutSlot[]) => {
      setState((prev) =>
        prev.composer ? { ...prev, composer: { ...prev.composer, layout: next } } : prev,
      );
      scheduleSave(next);
    },
    [scheduleSave],
  );

  const composer = state.composer;
  const selectedSlot = useMemo(() => {
    if (!composer || !selectedInput) return null;
    return composer.layout.find((s) => s.input === selectedInput) ?? null;
  }, [composer, selectedInput]);

  const selectedSlotIndex = useMemo(() => {
    if (!composer || !selectedInput) return -1;
    return composer.layout.findIndex((s) => s.input === selectedInput);
  }, [composer, selectedInput]);

  const onChangeSlot = useCallback(
    (next: LayoutSlot) => {
      if (!composer) return;
      const nextLayout = composer.layout.map((s) => (s.input === next.input ? next : s));
      handleLayoutChange(nextLayout);
    },
    [composer, handleLayoutChange],
  );

  const onChangeInputRef = useCallback(
    (nextRef: string) => {
      if (!composer || !selectedInput) return;
      const nextLayout = composer.layout.map((s) =>
        s.input === selectedInput ? { ...s, input: nextRef } : s,
      );
      setSelectedInput(nextRef);
      handleLayoutChange(nextLayout);
    },
    [composer, handleLayoutChange, selectedInput],
  );

  const onBringToFront = useCallback(() => {
    if (!composer || selectedSlotIndex < 0) return;
    const next = [...composer.layout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.push(moved);
    handleLayoutChange(next);
  }, [composer, handleLayoutChange, selectedSlotIndex]);

  const onSendToBack = useCallback(() => {
    if (!composer || selectedSlotIndex < 0) return;
    const next = [...composer.layout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.unshift(moved);
    handleLayoutChange(next);
  }, [composer, handleLayoutChange, selectedSlotIndex]);

  return (
    <DashboardLayout onLogout={logout}>
      <div className="px-6 py-4 space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold text-fg">Composer Layout</h1>
            <p className="text-sm text-fg-subtle font-mono">{id}</p>
          </div>
          <Button
            type="button"
            size="SM"
            theme="light"
            text="Back"
            onClick={() => navigate(-1)}
          />
        </div>

        {state.loading && (
          <div className="text-sm text-fg-subtle">Loading composer…</div>
        )}
        {state.error && (
          <div className="rounded-md border border-danger bg-danger-soft p-3 text-sm text-danger-soft-fg">
            {state.error}
          </div>
        )}

        {composer && (
          <div className="grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-4">
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-4 text-xs text-fg-muted">
                <span className="font-mono">
                  {composer.canvas.w}×{composer.canvas.h}
                </span>
                <Checkbox
                  label="Snap to grid"
                  checked={snapToGrid}
                  onChange={(e) => setSnapToGrid(e.target.checked)}
                />
                <div className="flex items-center gap-2">
                  <label htmlFor="grid-size" className="text-fg-muted">
                    Grid
                  </label>
                  <InputField
                    id="grid-size"
                    type="number"
                    min={1}
                    max={200}
                    value={gridSize}
                    onChange={(e) => {
                      const v = Number.parseInt(e.target.value, 10);
                      if (!Number.isNaN(v) && v > 0) setGridSize(v);
                    }}
                    className="w-20"
                  />
                </div>
                <Checkbox
                  label="Rulers"
                  checked={showRulers}
                  onChange={(e) => setShowRulers(e.target.checked)}
                />
                {saving && <span className="text-fg-subtle">saving…</span>}
              </div>
              <CanvasEditor
                canvas={composer.canvas}
                inputs={composer.inputs}
                layout={composer.layout}
                selectedInput={selectedInput}
                onSelect={setSelectedInput}
                onLayoutChange={handleLayoutChange}
                gridSize={gridSize}
                snapToGrid={snapToGrid}
                showRulers={showRulers}
                saving={saving}
              />
            </div>
            <div className="space-y-3">
              <LayoutSlotInspector
                slot={selectedSlot}
                canvas={composer.canvas}
                inputs={composer.inputs}
                slotIndex={selectedSlotIndex}
                layoutLength={composer.layout.length}
                onChange={onChangeSlot}
                onChangeInputRef={onChangeInputRef}
                onBringToFront={onBringToFront}
                onSendToBack={onSendToBack}
                saving={saving}
              />
              {composer.layout.length > 0 && (
                <div className="rounded-md border border-border bg-surface p-3">
                  <h4 className="text-xs font-semibold text-fg-muted uppercase tracking-wide mb-2">
                    Slots
                  </h4>
                  <ul className="space-y-1">
                    {composer.layout.map((slot) => (
                      <li key={slot.input}>
                        <button
                          type="button"
                          onClick={() => setSelectedInput(slot.input)}
                          className={`w-full text-left px-2 py-1 rounded-sm font-mono text-xs ${
                            slot.input === selectedInput
                              ? 'bg-accent-soft text-accent-soft-fg'
                              : 'text-fg-muted hover:bg-surface-muted'
                          }`}
                        >
                          {slot.input}
                        </button>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </DashboardLayout>
  );
}

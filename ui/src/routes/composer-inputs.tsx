import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import { useAuthStore } from '../hooks/useAuthStore';
import {
  useComposerStore,
  type Composer,
  type ComposerEffect,
  type ComposerInput,
} from '../hooks/useComposerStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { Card } from '../components/Card';
import { Button } from '../components/Button';
import { InputsList } from '../components/composers/InputsList';
import { InputRefPicker } from '../components/composers/InputRefPicker';
import { EffectEditor } from '../components/composers/EffectEditor';
import { API_BASE_URL } from '../lib/api';
import { getAuthCredentials } from '../lib/auth';

// Backend endpoints owned by B6. Called via raw fetch until U1 regenerates
// the typed client.
const COMPOSER_GET = (id: string) => `/api/composers/${encodeURIComponent(id)}`;
const COMPOSER_INPUTS = (id: string) => `/api/composers/${encodeURIComponent(id)}/inputs`;
const COMPOSER_INPUT_EFFECT = (id: string, ref: string) =>
  `/api/composers/${encodeURIComponent(id)}/inputs/${encodeURIComponent(ref)}/effect`;
const SOURCES_LIST = '/api/sources';

async function authedFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const credentials = getAuthCredentials();
  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  if (credentials) headers.set('Authorization', `Basic ${credentials}`);
  return fetch(`${API_BASE_URL}${path}`, { ...init, headers });
}

async function unwrapJSON<T>(res: Response, fallbackMsg: string): Promise<T> {
  if (!res.ok) {
    let detail = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as { detail?: string };
      if (body.detail) detail = body.detail;
    } catch {
      // ignore parse errors — use the status text
    }
    throw new Error(detail || fallbackMsg);
  }
  return (await res.json()) as T;
}

interface SourceListItem {
  id: string;
  device?: string;
  test_mode?: boolean;
}

interface SourcesResponse {
  sources?: SourceListItem[];
}

export default function ComposerInputs() {
  const { composerId } = useParams<{ composerId: string }>();
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  const setComposer = useComposerStore((s) => s.setComposer);
  const setAvailableSources = useComposerStore((s) => s.setAvailableSources);
  const upsertInputAction = useComposerStore((s) => s.upsertInput);
  const removeInputAction = useComposerStore((s) => s.removeInput);
  const setInputEffectAction = useComposerStore((s) => s.setInputEffect);

  const composer = useComposerStore((s) =>
    composerId ? s.composersById[composerId] : undefined,
  );
  const availableSources = useComposerStore((s) => s.availableSources);

  const [editingRef, setEditingRef] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(false);
  const [storeError, setStoreError] = useState<string | null>(null);

  const handleLogout = useCallback(() => logout(), [logout]);

  const refresh = useCallback(async () => {
    if (!composerId) return;
    setLoading(true);
    setStoreError(null);
    try {
      const [composerRes, sourcesRes] = await Promise.all([
        authedFetch(COMPOSER_GET(composerId)),
        authedFetch(SOURCES_LIST),
      ]);
      const composerData = await unwrapJSON<Composer>(composerRes, 'Failed to load composer');
      const sourcesData = await unwrapJSON<SourcesResponse>(
        sourcesRes,
        'Failed to load sources',
      );
      setComposer(composerData);
      setAvailableSources(
        (sourcesData.sources ?? []).map((s) => ({
          id: s.id,
          label: s.test_mode ? `${s.id} (test pattern)` : (s.device ?? s.id),
        })),
      );
    } catch (error) {
      setStoreError(error instanceof Error ? error.message : 'Failed to load composer');
    } finally {
      setLoading(false);
    }
  }, [composerId, setComposer, setAvailableSources]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleAddInput = useCallback(
    async (ref: string) => {
      if (!composerId) return;
      try {
        const res = await authedFetch(COMPOSER_INPUTS(composerId), {
          method: 'POST',
          body: JSON.stringify({ ref }),
        });
        await unwrapJSON<ComposerInput>(res, 'Failed to add input');
        upsertInputAction(composerId, { ref });
        toast.success(`Added ${ref}`);
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Failed to add input');
      }
    },
    [composerId, upsertInputAction],
  );

  const handleRemoveInput = useCallback(
    async (ref: string) => {
      if (!composerId) return;
      try {
        const res = await authedFetch(
          `${COMPOSER_INPUTS(composerId)}/${encodeURIComponent(ref)}`,
          { method: 'DELETE' },
        );
        if (!res.ok && res.status !== 204) {
          await unwrapJSON(res, 'Failed to remove input');
        }
        removeInputAction(composerId, ref);
        if (editingRef === ref) setEditingRef(null);
        toast.success(`Removed ${ref}`);
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Failed to remove input');
      }
    },
    [composerId, editingRef, removeInputAction],
  );

  const handleSaveEffect = useCallback(
    async (ref: string, effect: ComposerEffect | null) => {
      if (!composerId) return;
      setSaving(true);
      try {
        const res = await authedFetch(COMPOSER_INPUT_EFFECT(composerId, ref), {
          method: 'PATCH',
          body: JSON.stringify({ effect }),
        });
        if (!res.ok && res.status !== 204) {
          await unwrapJSON(res, 'Failed to save effect');
        }
        setInputEffectAction(composerId, ref, effect);
      } finally {
        setSaving(false);
      }
    },
    [composerId, setInputEffectAction],
  );

  const editingInput: ComposerInput | undefined = useMemo(() => {
    if (!editingRef || !composer) return undefined;
    return composer.inputs.find((i) => i.ref === editingRef);
  }, [editingRef, composer]);

  if (!composerId) {
    return (
      <DashboardLayout onLogout={handleLogout}>
        <DashboardLayout.MainContent>
          <Card className="p-6 text-danger">Missing composer id in URL.</Card>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  const canvasW = composer?.canvas.w ?? 1920;
  const canvasH = composer?.canvas.h ?? 1080;
  const existingRefs = composer?.inputs.map((i) => i.ref) ?? [];

  return (
    <DashboardLayout onLogout={handleLogout}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h1 className="text-2xl font-semibold text-fg">Composer inputs</h1>
              <p className="text-sm text-fg-muted">
                Composer <span className="font-mono">{composerId}</span> — canvas {canvasW}×{canvasH}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button theme="light" size="SM" text="Back" onClick={() => navigate(-1)} />
              <Button
                theme="light"
                size="SM"
                text={loading ? 'Refreshing...' : 'Refresh'}
                onClick={refresh}
                disabled={loading}
              />
            </div>
          </div>

          {storeError && (
            <Card className="p-4 border-danger text-danger">{storeError}</Card>
          )}

          {!composer && loading && (
            <Card className="p-6 text-fg-muted">Loading composer...</Card>
          )}

          {composer && (
            <>
              <Card className="p-4 space-y-4">
                <h2 className="text-base font-medium text-fg">Add input</h2>
                <InputRefPicker
                  availableSources={availableSources}
                  existingRefs={existingRefs}
                  disabled={loading}
                  onAdd={handleAddInput}
                />
              </Card>

              <Card className="p-4 space-y-3">
                <h2 className="text-base font-medium text-fg">Inputs</h2>
                <InputsList
                  inputs={composer.inputs}
                  editingRef={editingRef}
                  disabled={loading || saving}
                  onEdit={(ref) => setEditingRef((cur) => (cur === ref ? null : ref))}
                  onRemove={handleRemoveInput}
                />
              </Card>

              {editingInput && (
                <EffectEditor
                  composerId={composerId}
                  inputRef={editingInput.ref}
                  effect={editingInput.effect ?? null}
                  // No source-snapshot endpoint yet — backdrop is empty until
                  // B10 ships /api/sources/{id}/snapshot, then this becomes
                  // editingInput.ref → snapshot. Today the canvas still works
                  // for corner picking by clicking on the empty area.
                  snapshotStreamId={null}
                  inputWidth={canvasW}
                  inputHeight={canvasH}
                  saving={saving}
                  onSave={(effect) => handleSaveEffect(editingInput.ref, effect)}
                  onCancel={() => setEditingRef(null)}
                />
              )}
            </>
          )}
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

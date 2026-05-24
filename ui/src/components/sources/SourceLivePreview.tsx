import { useCallback, useEffect, useRef, useState } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { SectionHeader } from '../primitives/SectionHeader';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { api, unwrap, API_BASE_URL } from '../../lib/api';

interface SourceLivePreviewProps {
  sourceId: string;
  pollIntervalMs?: number;
}

interface SourceSnapshotResult {
  url: string;
}

// POST /api/sources/{id}/snapshot lands in B10; until then we fall back to
// the stream snapshot endpoint if a downstream stream shares the source-id,
// and otherwise surface a friendly placeholder so the layout still renders.
async function captureSnapshot(sourceId: string): Promise<SourceSnapshotResult> {
  const apiUnknown = api as unknown as {
    POST: (
      path: string,
      init: { params: { path: Record<string, string> } },
    ) => Promise<{ data?: { url: string }; error?: { detail?: string } }>;
  };
  try {
    const result = await apiUnknown.POST('/api/sources/{source_id}/snapshot', {
      params: { path: { source_id: sourceId } },
    });
    const data = unwrap(result, 'Failed to capture source snapshot');
    return { url: data.url };
  } catch (error) {
    // Fallback for pre-B10 daemons: try the stream snapshot endpoint with the
    // source id (works when a stream id happens to match the source id during
    // the migration window).
    const fallback = await api.POST('/api/streams/{stream_id}/snapshot', {
      params: { path: { stream_id: sourceId } },
    });
    if (fallback.error) {
      throw error instanceof Error ? error : new Error('Snapshot endpoint unavailable');
    }
    if (!fallback.data) {
      throw new Error('Empty snapshot response');
    }
    return { url: fallback.data.url };
  }
}

export function SourceLivePreview({ sourceId, pollIntervalMs = 1000 }: Readonly<SourceLivePreviewProps>) {
  const [imageUrl, setImageUrl] = useState<string | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const cancelledRef = useRef(false);

  const refresh = useCallback(async (): Promise<void> => {
    setLoading(true);
    try {
      const snap = await captureSnapshot(sourceId);
      if (cancelledRef.current) return;
      const fullUrl = snap.url.startsWith('http')
        ? snap.url
        : `${API_BASE_URL}${snap.url}`;
      // Bust the cache so the same path returns the latest pixels.
      setImageUrl(`${fullUrl}${fullUrl.includes('?') ? '&' : '?'}t=${Date.now()}`);
      setError(null);
    } catch (error_) {
      if (cancelledRef.current) return;
      setError(error_ instanceof Error ? error_.message : 'Snapshot failed');
    } finally {
      if (!cancelledRef.current) setLoading(false);
    }
  }, [sourceId]);

  useEffect(() => {
    cancelledRef.current = false;
    void refresh();
    return () => {
      cancelledRef.current = true;
    };
  }, [refresh]);

  useEffect(() => {
    if (!autoRefresh) return;
    const handle = window.setInterval(() => {
      void refresh();
    }, pollIntervalMs);
    return () => window.clearInterval(handle);
  }, [autoRefresh, pollIntervalMs, refresh]);

  return (
    <Card padding="lg">
      <SectionHeader
        title="Live preview"
        description={`Snapshot polled at ${(1000 / pollIntervalMs).toFixed(1)} Hz.`}
        actions={
          <>
            <Button
              text={autoRefresh ? 'Pause' : 'Resume'}
              theme="light"
              size="SM"
              onClick={() => setAutoRefresh((v) => !v)}
            />
            <Button text="Refresh" theme="light" size="SM" onClick={() => void refresh()} />
          </>
        }
      />
      <LivePreviewFrame
        {...(imageUrl !== undefined ? { src: imageUrl } : {})}
        loading={loading}
        error={error}
        alt={`Live preview of source ${sourceId}`}
      />
    </Card>
  );
}

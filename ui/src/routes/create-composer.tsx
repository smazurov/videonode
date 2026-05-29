import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import {
  ComposerCreationWizard,
  type ComposerWizardSource,
} from '../components/composers/ComposerCreationWizard';
import { useAuthStore } from '../hooks/useAuthStore';
import { API_BASE_URL } from '../lib/api';
import { getAuthCredentials } from '../lib/auth';

type SourcesResponse = { sources?: ComposerWizardSource[] };
type ComposersResponse = { composers?: { id: string }[] };

async function fetchJSON<T>(path: string): Promise<T> {
  const credentials = getAuthCredentials();
  const headers: Record<string, string> = {};
  if (credentials) headers.Authorization = `Basic ${credentials}`;
  const response = await fetch(`${API_BASE_URL}${path}`, { headers });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`);
  }
  return (await response.json()) as T;
}

// Stub bridge to U2's useSourceStore. Until that store lands, fetch
// /api/sources lazily here. When the store arrives, replace this hook
// with `useSourceStore((s) => s.sourcesList)` etc.
function useSourcesData(): {
  sources: ComposerWizardSource[];
  loading: boolean;
} {
  const [sources, setSources] = useState<ComposerWizardSource[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    fetchJSON<SourcesResponse>('/api/sources')
      .then((data) => {
        if (!cancelled) setSources(data.sources ?? []);
      })
      .catch(() => {
        if (!cancelled) setSources([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { sources, loading };
}

// Same stub story for the composer list — used only to validate ID
// uniqueness client-side. Real implementation will hang off useComposerStore.
function useExistingComposerIds(): string[] {
  const [ids, setIds] = useState<string[]>([]);
  useEffect(() => {
    let cancelled = false;
    fetchJSON<ComposersResponse>('/api/composers')
      .then((data) => {
        if (!cancelled) setIds((data.composers ?? []).map((c) => c.id));
      })
      .catch(() => {
        if (!cancelled) setIds([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);
  return ids;
}

export default function CreateComposer() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);
  const { sources, loading } = useSourcesData();
  const existingIds = useExistingComposerIds();

  const sortedSources = useMemo(
    () => [...sources].sort((a, b) => a.id.localeCompare(b.id)),
    [sources],
  );

  const handleSuccess = () => {
    navigate('/composers');
  };

  const handleCancel = () => {
    navigate('/composers');
  };

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                Create Composer
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Combine multiple sources onto a single canvas for downstream
                encodes.
              </p>
            </div>
            <Button
              theme="light"
              onClick={handleCancel}
              size="SM"
              text="Back to Composers"
            />
          </div>

          <ComposerCreationWizard
            sources={sortedSources}
            existingComposerIds={existingIds}
            sourcesLoading={loading}
            onSuccess={handleSuccess}
            onCancel={handleCancel}
          />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

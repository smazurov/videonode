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
import { apiComposers, apiSources, unwrap } from '../lib/api';

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
    apiSources
      .GET('/api/sources')
      .then((result) => {
        if (!cancelled) setSources(unwrap(result, 'Failed to load sources').sources ?? []);
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
    apiComposers
      .GET('/api/composers')
      .then((result) => {
        if (!cancelled) {
          setIds((unwrap(result, 'Failed to load composers').composers ?? []).map((c) => c.id));
        }
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

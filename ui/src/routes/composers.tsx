import { useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';

import { useAuthStore } from '../hooks/useAuthStore';
import { useComposerStore } from '../hooks/useComposerStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Spinner } from '../components/Spinner';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { ComposerList } from '../components/composers/ComposerList';
import type { ComposerData } from '../lib/composer-types';

export default function Composers() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);

  const composerIds = useComposerStore((s) => s.composerIds);
  const composersById = useComposerStore((s) => s.composersById);
  const { loading, error, lastUpdated } = useComposerStore(
    useShallow((s) => ({ loading: s.loading, error: s.error, lastUpdated: s.lastUpdated })),
  );
  const fetchComposers = useComposerStore((s) => s.fetchComposers);

  useEffect(() => {
    if (lastUpdated === null) void fetchComposers();
  }, [lastUpdated, fetchComposers]);

  // Bridge: API returns `id`; ComposerList consumes ComposerData with `composer_id`.
  const composers = useMemo<ComposerData[]>(
    () =>
      composerIds
        .map((id) => composersById[id])
        .filter((c): c is NonNullable<typeof c> => !!c)
        .map(({ id, inputs, layout, ...rest }) => ({
          composer_id: id,
          inputs: inputs ?? [],
          layout: layout ?? [],
          ...rest,
        })) as ComposerData[],
    [composerIds, composersById],
  );

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <Card padding="lg" className="space-y-6">
          <SectionHeader
            title="Composers"
            description="GLES BGRA compositors. Warm whenever at least one stream references them."
            actions={
              <Button
                theme="primary"
                size="SM"
                text="New composer"
                onClick={() => navigate('/composers/new')}
              />
            }
          />
          {error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft text-sm text-danger-soft-fg">
              {error}
            </div>
          )}
          {loading && lastUpdated === null ? (
            <div className="flex items-center justify-center py-12">
              <Spinner />
            </div>
          ) : (
            <ComposerList composers={composers} />
          )}
        </Card>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

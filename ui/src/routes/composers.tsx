import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import toast from 'react-hot-toast';
import { DocumentArrowUpIcon } from '@heroicons/react/24/outline';

import { useAuthStore } from '../hooks/useAuthStore';
import { useComposerStore } from '../hooks/useComposerStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Spinner } from '../components/Spinner';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { ComposerList } from '../components/composers/ComposerList';
import { ComposerImportDialog } from '../components/composers/ComposerImportDialog';
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
  const importComposerToml = useComposerStore((s) => s.importComposerToml);
  const [importOpen, setImportOpen] = useState(false);

  useEffect(() => {
    if (lastUpdated === null) void fetchComposers();
  }, [lastUpdated, fetchComposers]);

  const handleImport = async (toml: string) => {
    const composer = await importComposerToml(toml);
    toast.success(`Imported composer '${composer.id}'`);
    navigate(`/composers/${encodeURIComponent(composer.id)}`);
  };

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
              <div className="flex gap-2">
                <Button
                  theme="light"
                  size="SM"
                  text="Import TOML"
                  LeadingIcon={DocumentArrowUpIcon}
                  onClick={() => setImportOpen(true)}
                />
                <Button
                  theme="primary"
                  size="SM"
                  text="New composer"
                  onClick={() => navigate('/composers/new')}
                />
              </div>
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
        <ComposerImportDialog
          open={importOpen}
          title="Import composer from TOML"
          description="Paste or load a composer TOML document. The composer named in the document is created, or overwritten if it already exists."
          submitText="Import"
          onClose={() => setImportOpen(false)}
          onImport={handleImport}
        />
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

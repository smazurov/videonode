import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { SourceForm } from '../components/sources/SourceForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { Card } from '../components/Card';
import { useAuthStore } from '../hooks/useAuthStore';
import { useSourceStore } from '../hooks/useSourceStore';

export default function EditSource() {
  const navigate = useNavigate();
  const { sourceId } = useParams<{ sourceId: string }>();
  const { logout } = useAuthStore();

  const sourceData = useSourceStore((s) =>
    sourceId ? s.sourcesById[sourceId] : undefined,
  );
  const lastUpdated = useSourceStore((s) => s.lastUpdated);
  const fetchSources = useSourceStore((s) => s.fetchSources);
  const deleteSource = useSourceStore((s) => s.deleteSource);
  // Cross-reference lookup is best-effort: composer/stream stores may not be
  // loaded yet. Returns an empty list when no consumers are known.
  const getReferencesTo = (_id: string): { composers: string[]; streams: string[] } => ({
    composers: [],
    streams: [],
  });

  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  useEffect(() => {
    if (lastUpdated === null) {
      fetchSources();
    }
  }, [lastUpdated, fetchSources]);

  useEffect(() => {
    if (!sourceId || (lastUpdated !== null && !sourceData)) {
      navigate('/sources');
    }
  }, [sourceId, sourceData, lastUpdated, navigate]);

  if (!sourceId || lastUpdated === null || !sourceData) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 dark:border-white" />
      </div>
    );
  }

  const refs = getReferencesTo(sourceId);
  const hasBlockingRefs = refs.composers.length + refs.streams.length > 0;

  const handleSuccess = async () => {
    navigate('/sources');
  };

  const handleCancel = () => {
    navigate('/sources');
  };

  const handleDelete = async () => {
    if (hasBlockingRefs) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteSource(sourceId);
      navigate('/sources');
    } catch (error) {
      setDeleteError(error instanceof Error ? error.message : 'Failed to delete source');
    } finally {
      setDeleting(false);
    }
  };

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                Edit Source: {sourceData.id}
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Update the configuration for this source.
              </p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Sources" />
          </div>

          <SourceForm
            key={sourceData.id}
            initialData={sourceData}
            onSuccess={handleSuccess}
            onCancel={handleCancel}
          />

          <Card>
            <Card.Header>
              <h2 className="text-lg font-semibold text-fg">Danger zone</h2>
            </Card.Header>
            <Card.Content>
              {hasBlockingRefs && (
                <div className="mb-4 p-3 border border-warning rounded-md bg-warning-soft">
                  <p className="text-sm text-warning-soft-fg font-medium">
                    This source is in use and cannot be deleted.
                  </p>
                  <ul className="mt-2 text-xs text-warning-soft-fg list-disc list-inside space-y-1">
                    {refs.composers.map((id) => (
                      <li key={`c-${id}`}>composer: {id}</li>
                    ))}
                    {refs.streams.map((id) => (
                      <li key={`s-${id}`}>stream: {id}</li>
                    ))}
                  </ul>
                </div>
              )}

              {deleteError && (
                <div className="mb-4 p-3 border border-danger rounded-md bg-danger-soft">
                  <p className="text-sm text-danger-soft-fg">{deleteError}</p>
                </div>
              )}

              {!deleteConfirm ? (
                <Button
                  type="button"
                  theme="danger"
                  size="MD"
                  onClick={() => setDeleteConfirm(true)}
                  disabled={hasBlockingRefs || deleting}
                  text="Delete source"
                />
              ) : (
                <div className="space-y-3">
                  <p className="text-sm text-fg">
                    Permanently delete source{' '}
                    <span className="font-mono font-medium">{sourceData.id}</span>?
                    This cannot be undone.
                  </p>
                  <div className="flex items-center gap-2">
                    <Button
                      type="button"
                      theme="danger"
                      size="MD"
                      onClick={handleDelete}
                      disabled={deleting}
                      loading={deleting}
                      text="Confirm delete"
                    />
                    <Button
                      type="button"
                      theme="light"
                      size="MD"
                      onClick={() => setDeleteConfirm(false)}
                      disabled={deleting}
                      text="Cancel"
                    />
                  </div>
                </div>
              )}
            </Card.Content>
          </Card>
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

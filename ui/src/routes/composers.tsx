import { useCallback, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import { useAuthStore } from '../hooks/useAuthStore';
import { useComposerStore } from '../hooks/useComposerStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Spinner } from '../components/Spinner';
import { ComposerList } from '../components/composers/ComposerList';

export default function Composers() {
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  const composerIds = useComposerStore((state) => state.composerIds);
  const composersById = useComposerStore((state) => state.composersById);

  const { loading, error } = useComposerStore(
    useShallow((state) => ({ loading: state.loading, error: state.error })),
  );

  const fetchComposers = useComposerStore((state) => state.fetchComposers);

  useEffect(() => {
    fetchComposers();
  }, [fetchComposers]);

  const handleLogout = useCallback(() => logout(), [logout]);
  const handleCreate = useCallback(() => navigate('/composers/new'), [navigate]);

  const composers = composerIds
    .map((id) => composersById[id])
    .filter((c): c is NonNullable<typeof c> => !!c);

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-fg">Composers</h1>
            <p className="mt-1 text-sm text-fg-muted">
              GLES BGRA composition of N sources onto a canvas.
            </p>
          </div>
          <Button text="New composer" theme="primary" size="SM" onClick={handleCreate} />
        </div>

        {error && (
          <div className="rounded border border-danger-soft bg-danger-soft p-3 text-sm text-danger-soft-fg">
            {error}
          </div>
        )}

        {loading && composers.length === 0 ? (
          <div className="flex items-center justify-center p-12">
            <Spinner />
          </div>
        ) : (
          <ComposerList composers={composers} />
        )}
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

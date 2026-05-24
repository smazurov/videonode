import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import { useShallow } from 'zustand/shallow';
import { useAuthStore } from '../hooks/useAuthStore';
import { useComposerStore } from '../hooks/useComposerStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button, LinkButton } from '../components/Button';
import { Spinner } from '../components/Spinner';
import { EntityDetailLayout } from '../components/EntityDetailLayout';
import { ComposerOverviewPanel } from '../components/composers/ComposerOverviewPanel';
import { ComposerInputsPanel } from '../components/composers/ComposerInputsPanel';
import { ComposerConsumersPanel } from '../components/composers/ComposerConsumersPanel';

export default function ComposerDetail() {
  const navigate = useNavigate();
  const { composerId = '' } = useParams<{ composerId: string }>();
  const { logout } = useAuthStore();

  const composer = useComposerStore((state) => state.composersById[composerId]);
  const fetchComposers = useComposerStore((state) => state.fetchComposers);
  const deleteComposer = useComposerStore((state) => state.deleteComposer);

  const { loading, error } = useComposerStore(
    useShallow((state) => ({ loading: state.loading, error: state.error })),
  );

  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (!composer) {
      fetchComposers();
    }
  }, [composer, fetchComposers]);

  const handleLogout = useCallback(() => logout(), [logout]);

  const downstreamCount = composer?.downstream_stream_ids?.length ?? 0;

  const handleDelete = useCallback(async () => {
    if (!composer) return;
    if (downstreamCount > 0) {
      const plural = downstreamCount === 1 ? '' : 's';
      toast.error(
        `Cannot delete: ${downstreamCount} stream${plural} reference this composer.`,
      );
      return;
    }
    const confirmed = window.confirm(
      `Delete composer "${composer.composer_id}"? This cannot be undone.`,
    );
    if (!confirmed) return;
    setDeleting(true);
    try {
      await deleteComposer(composer.composer_id);
      toast.success(`Composer "${composer.composer_id}" deleted.`);
      navigate('/composers');
    } catch (error_) {
      toast.error(error_ instanceof Error ? error_.message : 'Failed to delete composer');
    } finally {
      setDeleting(false);
    }
  }, [composer, deleteComposer, downstreamCount, navigate]);

  if (!composer && loading) {
    return (
      <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
        <DashboardLayout.MainContent>
          <div className="flex items-center justify-center p-12">
            <Spinner />
          </div>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  if (!composer) {
    return (
      <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
        <DashboardLayout.MainContent>
          <div className="rounded-lg border border-border bg-surface-raised p-8 text-center">
            <p className="text-sm text-fg">
              Composer <span className="font-mono">{composerId}</span> not found.
            </p>
            {error && <p className="mt-2 text-xs text-danger">{error}</p>}
            <div className="mt-4">
              <LinkButton to="/composers" text="Back to composers" theme="light" size="SM" />
            </div>
          </div>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  const downstreamPlural = downstreamCount === 1 ? '' : 's';
  const inputsPlural = composer.inputs.length === 1 ? '' : 's';
  const deleteTitle =
    downstreamCount > 0
      ? `Blocked: ${downstreamCount} downstream stream${downstreamPlural} still reference this composer`
      : 'Delete this composer';
  const subtitle = `${composer.inputs.length} input${inputsPlural} · ${downstreamCount} downstream stream${downstreamPlural}`;

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <EntityDetailLayout
          breadcrumbs={[
            { label: 'Composers', to: '/composers' },
            { label: composer.composer_id },
          ]}
          title={composer.composer_id}
          subtitle={subtitle}
          actions={
            <>
              <LinkButton
                to={`/composers/${encodeURIComponent(composer.composer_id)}/layout`}
                text="Edit layout"
                theme="light"
                size="SM"
              />
              <LinkButton
                to={`/composers/${encodeURIComponent(composer.composer_id)}/inputs`}
                text="Edit inputs"
                theme="light"
                size="SM"
              />
              <Button
                text="Delete"
                theme="danger"
                size="SM"
                onClick={handleDelete}
                disabled={deleting || downstreamCount > 0}
                loading={deleting}
                title={deleteTitle}
              />
            </>
          }
        >
          <ComposerOverviewPanel composer={composer} />
          <ComposerInputsPanel composer={composer} />
          <ComposerConsumersPanel composer={composer} />
        </EntityDetailLayout>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

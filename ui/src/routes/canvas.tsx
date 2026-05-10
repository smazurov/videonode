import { useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { CanvasForm } from '../components/CanvasForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';

export default function CanvasRoute() {
  const navigate = useNavigate();
  const { streamId } = useParams<{ streamId?: string }>();
  const { logout } = useAuthStore();

  const isEdit = !!streamId;
  const streamData = useStreamStore((state) => (streamId ? state.streamsById[streamId] : undefined));
  const lastUpdated = useStreamStore((state) => state.lastUpdated);
  const fetchStreams = useStreamStore((state) => state.fetchStreams);

  useEffect(() => {
    if (lastUpdated === null) {
      fetchStreams();
    }
  }, [lastUpdated, fetchStreams]);

  useEffect(() => {
    if (isEdit && lastUpdated !== null && !streamData) {
      navigate('/streams');
    }
  }, [isEdit, streamData, lastUpdated, navigate]);

  if (isEdit && (lastUpdated === null || !streamData)) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 dark:border-white" />
      </div>
    );
  }

  const handleSuccess = async () => {
    navigate('/streams');
  };

  const handleCancel = () => {
    navigate('/streams');
  };

  const title = isEdit ? `Edit Canvas: ${streamData?.stream_id}` : 'Create Canvas Stream';
  const subtitle = isEdit
    ? 'Update this composite canvas. Changes apply on next restart.'
    : 'Composite 1–4 individual streams into a single encoded canvas.';

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">{title}</h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">{subtitle}</p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Streams" />
          </div>

          <CanvasForm
            {...(streamData ? { initialData: streamData } : {})}
            onSuccess={handleSuccess}
            onCancel={handleCancel}
          />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

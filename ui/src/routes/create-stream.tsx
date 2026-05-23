import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { StreamForm } from '../components/StreamForm';
import { CanvasForm } from '../components/CanvasForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';

// Post-pipeline-rip: canvases and single-source streams are unified
// under the daemon's pipeline.Stream model — a canvas is just a
// stream with N>1 inputs. The UI mirrors that by combining both
// creation flows under a single /streams/new route, with an inline
// type picker. The dedicated /streams/canvas/new route is preserved
// for back-compat with bookmarks but redirects here via main.tsx.
type StreamType = 'single' | 'multi';

export default function CreateStream() {
  const navigate = useNavigate();
  const { logout } = useAuthStore();
  const [type, setType] = useState<StreamType>('single');

  const handleSuccess = async () => {
    navigate('/streams');
  };

  const handleCancel = () => {
    navigate('/streams');
  };

  const handleLogout = () => {
    logout();
  };

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                Create New Stream
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Configure a new video stream — single-source from one capture
                device, or multi-source composited from 1&ndash;4 streams.
              </p>
            </div>
            <Button
              theme="light"
              onClick={handleCancel}
              size="SM"
              text="Back to Streams"
            />
          </div>

          <div
            role="radiogroup"
            aria-label="Stream type"
            className="flex gap-2 border border-gray-200 dark:border-gray-700 rounded-md p-1 w-fit"
          >
            <button
              type="button"
              role="radio"
              aria-checked={type === 'single'}
              onClick={() => setType('single')}
              className={
                'px-4 py-1.5 text-sm rounded-sm transition-colors ' +
                (type === 'single'
                  ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
                  : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800')
              }
            >
              Single source
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={type === 'multi'}
              onClick={() => setType('multi')}
              className={
                'px-4 py-1.5 text-sm rounded-sm transition-colors ' +
                (type === 'multi'
                  ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
                  : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800')
              }
            >
              Multi-source (canvas)
            </button>
          </div>

          {type === 'single' ? (
            <StreamForm onSuccess={handleSuccess} onCancel={handleCancel} />
          ) : (
            <CanvasForm onSuccess={handleSuccess} onCancel={handleCancel} />
          )}
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

import { useCallback, useState } from 'react';
import { BeakerIcon, CheckIcon } from '@heroicons/react/24/outline';
import toast from 'react-hot-toast';
import { useShallow } from 'zustand/react/shallow';
import { api } from '../../lib/api';
import { useStreamStore } from '../../hooks/useStreamStore';
import { ICON_SIZE } from '../../utils';
import { MenuRow } from './MenuRow';

interface TestModeMenuItemProps {
  streamId: string;
  onAfterToggle?: () => void;
}

export function TestModeMenuItem({
  streamId,
  onAfterToggle,
}: Readonly<TestModeMenuItemProps>) {
  const { testMode, hasCustomCommand } = useStreamStore(
    useShallow((state) => ({
      testMode: state.streamsById[streamId]?.test_mode ?? false,
      hasCustomCommand: !!state.streamsById[streamId]?.custom_ffmpeg_command,
    })),
  );
  const [isToggling, setIsToggling] = useState(false);

  const handleToggle = useCallback(async () => {
    if (isToggling) return;
    setIsToggling(true);
    try {
      const { error } = await api.PATCH('/api/streams/{stream_id}', {
        params: { path: { stream_id: streamId } },
        body: { test_mode: !testMode },
      });
      if (error) throw new Error(error.detail ?? 'Failed to toggle test mode');
      onAfterToggle?.();
    } catch (error) {
      console.error('Failed to toggle test mode:', error);
      toast.error('Failed to toggle test mode');
    } finally {
      setIsToggling(false);
    }
  }, [isToggling, streamId, testMode, onAfterToggle]);

  const title = (() => {
    if (hasCustomCommand) return 'Test mode disabled when custom command is set';
    if (testMode) return 'Disable Test Mode';
    return 'Enable Test Mode';
  })();

  return (
    <MenuRow
      icon={BeakerIcon}
      label="Test Mode"
      onClick={handleToggle}
      disabled={isToggling || hasCustomCommand}
      title={title}
      trailing={testMode ? <CheckIcon className={`${ICON_SIZE.SM} shrink-0 text-accent`} /> : null}
    />
  );
}

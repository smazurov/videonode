import { useCallback, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { Menu, MenuButton, MenuItems } from '@headlessui/react';
import {
  ArrowPathIcon,
  CodeBracketIcon,
  DocumentTextIcon,
  PencilSquareIcon,
  TrashIcon,
  ViewfinderCircleIcon,
} from '@heroicons/react/24/outline';
import { EllipsisVerticalIcon } from '@heroicons/react/24/solid';
import { Button } from './Button';
import { TestModeMenuItem } from './menu/TestModeMenuItem';
import { MenuRow } from './menu/MenuRow';
import { MENU_DOTS_BUTTON_CLASS } from './menu/menuStyles';
import { api } from '../lib/api';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager, type TaggedStreamStateChangedEvent } from '../hooks/useSSEManager';
import { ICON_SIZE } from '../utils';

interface StreamCardActionsProps {
  streamId: string;
  onDelete?: ((streamId: string) => void) | undefined;
  onShowFFmpegSheet: () => void;
  onShowLogsSheet: () => void;
  onShowPerspectiveSheet: () => void;
  onRequestPlayerRefresh: () => void;
}

const RESTART_TIMEOUT_MS = 10_000;

function useWaitForStreamRunning(streamId: string) {
  const pendingRef = useRef<{
    resolve: () => void;
    reject: (err: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  } | null>(null);

  const settle = useCallback((err?: Error) => {
    const pending = pendingRef.current;
    if (!pending) return;
    clearTimeout(pending.timer);
    pendingRef.current = null;
    if (err) pending.reject(err);
    else pending.resolve();
  }, []);

  const handler = useCallback(
    (event: TaggedStreamStateChangedEvent) => {
      if (event.stream_id === streamId && event.action === 'running') {
        settle();
      }
    },
    [streamId, settle]
  );

  useSSEManager({ onStreamStateEvent: handler });

  return useCallback(
    () =>
      new Promise<void>((resolve, reject) => {
        // Cancel any prior pending wait — only the latest restart is relevant.
        if (pendingRef.current) {
          clearTimeout(pendingRef.current.timer);
          pendingRef.current.reject(new Error('superseded'));
        }
        const timer = setTimeout(() => {
          settle(new Error('timeout waiting for stream to become running'));
        }, RESTART_TIMEOUT_MS);
        pendingRef.current = { resolve, reject, timer };
      }),
    [settle]
  );
}

function EditButton({ streamId }: { readonly streamId: string }) {
  const navigate = useNavigate();

  return (
    <Button
      theme="blank"
      size="SM"
      onClick={() => navigate(`/streams/${streamId}/edit`)}
      title="Edit Stream"
      LeadingIcon={PencilSquareIcon}
    />
  );
}

function RestartButton({
  streamId,
  onRequestPlayerRefresh,
}: {
  readonly streamId: string;
  readonly onRequestPlayerRefresh: () => void;
}) {
  const [isRestarting, setIsRestarting] = useState(false);
  const waitForRunning = useWaitForStreamRunning(streamId);

  const handleRestart = async () => {
    setIsRestarting(true);

    const wait = waitForRunning();
    // Swallow the wait promise's rejection independently — we only care about
    // unhandled rejections, not propagating "superseded" / timeout errors past
    // the catch below.
    wait.catch(() => {});
    try {
      const { error } = await api.POST('/api/streams/{stream_id}/restart', {
        params: { path: { stream_id: streamId } },
      });
      if (error) throw new Error(error.detail ?? 'Failed to restart stream');

      await wait;
      onRequestPlayerRefresh();
    } catch (error) {
      console.error('Failed to restart stream:', error);
      toast.error('Failed to restart stream');
    } finally {
      setIsRestarting(false);
    }
  };

  return (
    <Button
      theme="blank"
      size="SM"
      onClick={handleRestart}
      disabled={isRestarting}
      title="Restart Stream"
      LeadingIcon={ArrowPathIcon}
    />
  );
}

interface MoreActionsMenuProps {
  readonly streamId: string;
  readonly onDelete?: ((streamId: string) => void) | undefined;
  readonly onShowFFmpegSheet: () => void;
  readonly onShowLogsSheet: () => void;
  readonly onShowPerspectiveSheet: () => void;
  readonly onRequestPlayerRefresh: () => void;
}

function MoreActionsMenu({
  streamId,
  onDelete,
  onShowFFmpegSheet,
  onShowLogsSheet,
  onShowPerspectiveSheet,
  onRequestPlayerRefresh,
}: MoreActionsMenuProps) {
  const hasCustomCommand = useStreamStore(
    (state) => !!state.streamsById[streamId]?.custom_ffmpeg_command,
  );
  const [isDeleting, setIsDeleting] = useState(false);
  const waitForRunning = useWaitForStreamRunning(streamId);

  const handleTestModeAfterToggle = useCallback(async () => {
    const wait = waitForRunning();
    await wait;
    onRequestPlayerRefresh();
  }, [waitForRunning, onRequestPlayerRefresh]);

  const handleDelete = async () => {
    if (!onDelete || isDeleting) return;

    setIsDeleting(true);
    try {
      await onDelete(streamId);
    } catch (error) {
      console.error('Failed to delete stream:', error);
    } finally {
      setIsDeleting(false);
    }
  };
  return (
    <Menu>
      <MenuButton title="More actions" className={MENU_DOTS_BUTTON_CLASS}>
        <EllipsisVerticalIcon className={`${ICON_SIZE.SM} shrink-0`} />
      </MenuButton>
      <MenuItems
        anchor="bottom end"
        className="z-50 mt-1 min-w-[220px] rounded border border-border bg-surface-raised py-1 shadow-lg focus:outline-none"
      >
        <MenuRow
          icon={CodeBracketIcon}
          label={hasCustomCommand ? 'Custom FFmpeg Command' : 'FFmpeg Command'}
          onClick={onShowFFmpegSheet}
          trailing={hasCustomCommand ? <span className="h-2 w-2 rounded-full bg-accent" aria-hidden="true" /> : null}
        />
        <MenuRow icon={DocumentTextIcon} label="Stream Logs" onClick={onShowLogsSheet} />
        <MenuRow icon={ViewfinderCircleIcon} label="Perspective Calibration" onClick={onShowPerspectiveSheet} />

        <TestModeMenuItem streamId={streamId} onAfterToggle={handleTestModeAfterToggle} />

        {onDelete && (
          <MenuRow
            icon={TrashIcon}
            label="Delete Stream"
            onClick={handleDelete}
            variant="danger"
            disabled={isDeleting}
          />
        )}
      </MenuItems>
    </Menu>
  );
}

export function StreamCardActions({
  streamId,
  onDelete,
  onShowFFmpegSheet,
  onShowLogsSheet,
  onShowPerspectiveSheet,
  onRequestPlayerRefresh,
}: Readonly<StreamCardActionsProps>) {
  return (
    <div className="flex items-center space-x-1">
      <EditButton streamId={streamId} />
      <RestartButton
        streamId={streamId}
        onRequestPlayerRefresh={onRequestPlayerRefresh}
      />
      <MoreActionsMenu
        streamId={streamId}
        onDelete={onDelete}
        onShowFFmpegSheet={onShowFFmpegSheet}
        onShowLogsSheet={onShowLogsSheet}
        onShowPerspectiveSheet={onShowPerspectiveSheet}
        onRequestPlayerRefresh={onRequestPlayerRefresh}
      />
    </div>
  );
}

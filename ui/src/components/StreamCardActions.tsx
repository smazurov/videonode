import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { useShallow } from 'zustand/react/shallow';
import { Button } from './Button';
import { toggleTestMode, restartStream } from '../lib/api';
import { useStreamStore } from '../hooks/useStreamStore';

interface StreamCardActionsProps {
  streamId: string;
  onDelete?: ((streamId: string) => void) | undefined;
  onShowFFmpegSheet: () => void;
  onRequestPlayerRefresh: () => void;
}

function FFmpegCommandButton({ streamId, onShowSheet }: { readonly streamId: string; readonly onShowSheet: () => void }) {
  const hasCustomCommand = useStreamStore(
    (state) => !!state.streamsById[streamId]?.custom_ffmpeg_command
  );

  return (
    <Button
      theme={hasCustomCommand ? "primary" : "blank"}
      size="SM"
      onClick={onShowSheet}
      title={hasCustomCommand ? "Custom FFmpeg Command" : "FFmpeg Command"}
      LeadingIcon={({ className }) => (
        <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
        </svg>
      )}
    />
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
      LeadingIcon={({ className }) => (
        <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
        </svg>
      )}
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

  const handleRestart = async () => {
    setIsRestarting(true);

    try {
      await restartStream(streamId);

      await new Promise(resolve => setTimeout(resolve, 1500));

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
      LeadingIcon={({ className }) => (
        <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
      )}
    />
  );
}

function TestModeButton({
  streamId,
  onRequestPlayerRefresh,
}: {
  readonly streamId: string;
  readonly onRequestPlayerRefresh: () => void;
}) {
  const [isTogglingTestMode, setIsTogglingTestMode] = useState(false);
  const { testMode, hasCustomCommand } = useStreamStore(
    useShallow((state) => ({
      testMode: state.streamsById[streamId]?.test_mode ?? false,
      hasCustomCommand: !!state.streamsById[streamId]?.custom_ffmpeg_command,
    }))
  );

  const handleToggleTestMode = async () => {
    setIsTogglingTestMode(true);

    try {
      await toggleTestMode(streamId, !testMode);

      onRequestPlayerRefresh();
    } catch (error) {
      console.error('Failed to toggle test mode:', error);
      toast.error('Failed to toggle test mode');
    } finally {
      setIsTogglingTestMode(false);
    }
  };

  const title = (() => {
    if (hasCustomCommand) return "Test mode disabled when custom command is set";
    if (testMode) return "Disable Test Mode";
    return "Enable Test Mode";
  })();

  return (
    <Button
      theme={testMode ? "primary" : "blank"}
      size="SM"
      onClick={handleToggleTestMode}
      disabled={isTogglingTestMode || hasCustomCommand}
      title={title}
      LeadingIcon={({ className }) => (
        <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z" />
        </svg>
      )}
    />
  );
}

function DeleteButton({
  streamId,
  onDelete,
}: {
  readonly streamId: string;
  readonly onDelete: (streamId: string) => void;
}) {
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = async () => {
    if (isDeleting) return;

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
    <Button
      theme="danger"
      size="SM"
      onClick={handleDelete}
      disabled={isDeleting}
      title="Delete Stream"
      LeadingIcon={({ className }) => (
        <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
        </svg>
      )}
    />
  );
}

export function StreamCardActions({
  streamId,
  onDelete,
  onShowFFmpegSheet,
  onRequestPlayerRefresh,
}: Readonly<StreamCardActionsProps>) {
  return (
    <div className="flex items-center space-x-1">
      <FFmpegCommandButton streamId={streamId} onShowSheet={onShowFFmpegSheet} />
      <EditButton streamId={streamId} />
      <RestartButton
        streamId={streamId}
        onRequestPlayerRefresh={onRequestPlayerRefresh}
      />
      <TestModeButton
        streamId={streamId}
        onRequestPlayerRefresh={onRequestPlayerRefresh}
      />
      {onDelete && (
        <DeleteButton streamId={streamId} onDelete={onDelete} />
      )}
    </div>
  );
}

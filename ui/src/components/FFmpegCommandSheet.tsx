import { useState, useEffect, useCallback } from 'react';
import { DocumentDuplicateIcon } from '@heroicons/react/24/outline';
import { Button } from './Button';
import { IconButton } from './IconButton';
import { Badge } from './Badge';
import { BottomSheet } from './BottomSheet';
import { Spinner } from './Spinner';
import { Checkbox } from './Checkbox';
import type { components } from '../lib/api.generated';
import { api } from '../lib/api';

type FFmpegCommandData = components["schemas"]["FFmpegCommandData"];
import { useStreamStore } from '../hooks/useStreamStore';
import toast from 'react-hot-toast';

// Define encoder option groups
const ENCODER_GROUPS = [
  { group: 'Software', options: [
    { value: 'libx264', label: 'libx264' },
    { value: 'libx265', label: 'libx265' },
    { value: 'libvpx', label: 'libvpx' },
    { value: 'libvpx-vp9', label: 'libvpx-vp9' },
    { value: 'mpeg4', label: 'mpeg4' },
    { value: 'libxvid', label: 'libxvid' },
  ]},
  { group: 'VAAPI (Intel/AMD)', options: [
    { value: 'h264_vaapi', label: 'h264_vaapi' },
    { value: 'hevc_vaapi', label: 'hevc_vaapi' },
    { value: 'mpeg2_vaapi', label: 'mpeg2_vaapi' },
    { value: 'vp8_vaapi', label: 'vp8_vaapi' },
    { value: 'vp9_vaapi', label: 'vp9_vaapi' },
    { value: 'av1_vaapi', label: 'av1_vaapi' },
  ]},
  { group: 'RKMPP (Rockchip)', options: [
    { value: 'h264_rkmpp', label: 'h264_rkmpp' },
    { value: 'hevc_rkmpp', label: 'hevc_rkmpp' },
    { value: 'vp8_rkmpp', label: 'vp8_rkmpp' },
    { value: 'mjpeg_rkmpp', label: 'mjpeg_rkmpp' },
  ]},
] as const;

interface FFmpegCommandSheetProps {
  isOpen: boolean;
  onClose: () => void;
  streamId: string;
  onRefresh?: (streamId: string) => void;
}

type ViewMode = 'view' | 'edit';

interface CommandCache {
  base: FFmpegCommandData | null;
  overrides: Record<string, FFmpegCommandData>;
}

export function FFmpegCommandSheet({ isOpen, onClose, streamId, onRefresh }: Readonly<FFmpegCommandSheetProps>) {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [mode, setMode] = useState<ViewMode>('view');
  const [cache, setCache] = useState<CommandCache>({ base: null, overrides: {} });
  const [editedCommand, setEditedCommand] = useState('');
  const [selectedEncoder, setSelectedEncoder] = useState('');
  const [showEncoderOverride, setShowEncoderOverride] = useState(false);

  // Use selector to only re-render when THIS stream's custom_ffmpeg_command changes
  const customCommand = useStreamStore((state) => state.streamsById[streamId]?.custom_ffmpeg_command);

  // Get the currently displayed command based on mode and selections
  const getCurrentCommand = (): FFmpegCommandData | null => {
    if (showEncoderOverride && selectedEncoder && cache.overrides[selectedEncoder]) {
      return cache.overrides[selectedEncoder];
    }
    return cache.base;
  };

  const currentCommand = getCurrentCommand();

  // Load base command function with useCallback to prevent infinite loops
  const loadBaseCommand = useCallback(async () => {
    setLoading(true);
    try {
      const { data, error } = await api.GET("/api/streams/{stream_id}/ffmpeg", {
        params: { path: { stream_id: streamId } },
      });
      if (error) throw new Error(error.detail ?? 'Failed to fetch FFmpeg command');
      setCache(prev => ({ ...prev, base: data }));
    } catch (error) {
      console.error('Failed to fetch FFmpeg command:', error);
      toast.error('Failed to load FFmpeg command');
    } finally {
      setLoading(false);
    }
  }, [streamId]);

  // Reset state when opening/closing
  useEffect(() => {
    if (isOpen && streamId) {
      setMode('view');
      setSelectedEncoder('');
      setShowEncoderOverride(false);
      setEditedCommand('');
      loadBaseCommand();
    }
  }, [isOpen, streamId, loadBaseCommand]);



  // Load command with encoder override (with caching)
  const loadEncoderOverride = async (encoder: string) => {
    if (cache.overrides[encoder]) {
      return; // Already cached
    }

    setLoading(true);
    try {
      const { data, error } = await api.GET("/api/streams/{stream_id}/ffmpeg", {
        params: { path: { stream_id: streamId }, query: { override: encoder } },
      });
      if (error) throw new Error(error.detail ?? `Failed to load command for ${encoder}`);
      setCache(prev => ({
        ...prev,
        overrides: { ...prev.overrides, [encoder]: data }
      }));
    } catch (error) {
      console.error('Failed to fetch encoder override command:', error);
      toast.error(`Failed to load command for ${encoder}`);
    } finally {
      setLoading(false);
    }
  };

  // Handle encoder selection change
  const handleEncoderChange = async (encoder: string) => {
    setSelectedEncoder(encoder);
    if (encoder && encoder !== '' && !cache.overrides[encoder]) {
      await loadEncoderOverride(encoder);
    }
  };

  // Start editing current command
  const startEditing = () => {
    if (currentCommand) {
      // If stream has custom command, start with that; otherwise use current displayed command
      const initialCommand = customCommand || currentCommand.command;
      setEditedCommand(initialCommand);
      setMode('edit');
    }
  };

  // Cancel editing
  const cancelEditing = () => {
    setEditedCommand('');
    setMode('view');
  };

  // Save custom command
  const saveCommand = async () => {
    if (!editedCommand.trim()) {
      toast.error('Command cannot be empty');
      return;
    }

    setSaving(true);
    try {
      const { error } = await api.PATCH("/api/streams/{stream_id}", {
        params: { path: { stream_id: streamId } },
        body: { custom_ffmpeg_command: editedCommand, test_mode: false },
      });
      if (error) throw new Error(error.detail ?? 'Failed to save FFmpeg command');
      setMode('view');
      // Refresh base command to reflect the custom command
      await loadBaseCommand();
      // Refresh the stream card to show the custom command indicator
      if (onRefresh) {
        await onRefresh(streamId);
      }
      toast.success('FFmpeg command updated successfully');
    } catch (error) {
      console.error('Failed to save FFmpeg command:', error);
      toast.error('Failed to save FFmpeg command');
    } finally {
      setSaving(false);
    }
  };

  // Revert to auto-generated command
  const revertToAuto = async () => {
    setSaving(true);
    try {
      const { error } = await api.PATCH("/api/streams/{stream_id}", {
        params: { path: { stream_id: streamId } },
        body: { custom_ffmpeg_command: "" },
      });
      if (error) throw new Error(error.detail ?? 'Failed to clear custom command');
      // Clear cache and reload
      setCache({ base: null, overrides: {} });
      setSelectedEncoder('');
      setShowEncoderOverride(false);
      await loadBaseCommand();
      // Refresh the stream card to remove the custom command indicator
      if (onRefresh) {
        await onRefresh(streamId);
      }
      toast.success('Reverted to auto-generated command');
    } catch (error) {
      console.error('Failed to clear custom command:', error);
      toast.error('Failed to revert to auto-generated command');
    } finally {
      setSaving(false);
    }
  };

  // Copy command to clipboard
  const copyToClipboard = async () => {
    if (currentCommand?.command) {
      try {
        await navigator.clipboard.writeText(currentCommand.command);
        toast.success('Command copied to clipboard');
      } catch (error) {
        console.error('Failed to copy to clipboard:', error);
        toast.error('Failed to copy to clipboard');
      }
    }
  };



  return (
    <BottomSheet
      open={isOpen}
      onClose={onClose}
      title={`FFmpeg Command - ${streamId}`}
      maxWidth="4xl"
      headerExtra={currentCommand?.is_custom ? <Badge tone="warning">Custom</Badge> : undefined}
    >
      <>
                {/* Encoder Override (only in view mode and if not custom) */}
                {mode === 'view' && !currentCommand?.is_custom && (
                  <div className="mb-4">
                    <Checkbox
                      className="mb-2"
                      checked={showEncoderOverride}
                      onChange={(e) => {
                        setShowEncoderOverride(e.target.checked);
                        if (!e.target.checked) {
                          setSelectedEncoder('');
                        }
                      }}
                      label={<span className="text-sm font-medium text-fg">Override encoder</span>}
                    />

                    {showEncoderOverride && (
                      <select
                        value={selectedEncoder}
                        aria-label="Encoder override"
                        onChange={(e) => handleEncoderChange(e.target.value)}
                        className="block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent"
                        disabled={loading}
                      >
                        <option value="">Auto-select encoder</option>
                        {ENCODER_GROUPS.map((group) => (
                          <optgroup key={group.group} label={group.group}>
                            {group.options.map((option) => (
                              <option key={option.value} value={option.value}>
                                {option.label}
                              </option>
                            ))}
                          </optgroup>
                        ))}
                      </select>
                    )}
                  </div>
                )}

                {/* Content */}
                {(() => {
                  if (loading) {
                    return (
                      <div className="flex items-center justify-center h-48">
                        <Spinner size="lg" />
                      </div>
                    );
                  }

                  if (!currentCommand) {
                    return (
                      <div className="text-center py-8 text-fg-subtle">No command data available</div>
                    );
                  }

                  return (
                  <div className="space-y-4">
                    <div className="relative group">
                      {mode === 'edit' ? (
                        <textarea
                          value={editedCommand}
                          aria-label="FFmpeg command"
                          onChange={(e) => setEditedCommand(e.target.value)}
                          className="w-full h-48 p-4 font-mono text-sm text-fg bg-surface-muted border border-border rounded-lg resize-none focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent overflow-auto break-all"
                          placeholder="Enter FFmpeg command..."
                          disabled={saving}
                          spellCheck={false}
                          wrap="soft"
                        />
                      ) : (
                        <>
                          <pre className="w-full h-48 p-4 font-mono text-sm text-fg bg-surface-muted border border-border rounded-lg overflow-auto whitespace-pre-wrap break-all">
                            {currentCommand.command}
                          </pre>
                          <IconButton
                            icon={DocumentDuplicateIcon}
                            label="Copy to clipboard"
                            size="SM"
                            onClick={copyToClipboard}
                            className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity"
                          />
                        </>
                      )}
                    </div>

                    {/* Actions */}
                    <div className="flex justify-between">
                      <div className="flex space-x-2">
                        {mode === 'view' ? (
                          <>
                            <Button
                              theme="primary"
                              size="MD"
                              onClick={startEditing}
                              text="Edit Command"
                            />
                            {currentCommand.is_custom && (
                              <Button
                                theme="light"
                                size="MD"
                                onClick={revertToAuto}
                                disabled={saving}
                                text="Revert to Auto"
                              />
                            )}
                          </>
                        ) : (
                          <>
                            <Button
                              theme="primary"
                              size="MD"
                              onClick={saveCommand}
                              disabled={saving}
                              text={saving ? 'Saving...' : 'Save'}
                            />
                            <Button
                              theme="light"
                              size="MD"
                              onClick={cancelEditing}
                              disabled={saving}
                              text="Cancel"
                            />
                          </>
                        )}
                      </div>
                      <Button
                        theme="light"
                        size="MD"
                        onClick={onClose}
                        text="Close"
                      />
                    </div>
                  </div>
                  );
                })()}
      </>
    </BottomSheet>
  );
}
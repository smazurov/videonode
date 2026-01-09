import { useState, useEffect, useCallback } from 'react';
import { Dialog, DialogPanel, DialogTitle, Transition, TransitionChild } from '@headlessui/react';
import { Button } from './Button';
import { getFFmpegCommand, setFFmpegCommand, clearFFmpegCommand, FFmpegCommandData } from '../lib/api';
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
      const data = await getFFmpegCommand(streamId);
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
      const data = await getFFmpegCommand(streamId, encoder);
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
      await setFFmpegCommand(streamId, editedCommand);
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
      await clearFFmpegCommand(streamId);
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
    <Transition show={isOpen}>
      <Dialog onClose={onClose} className="relative z-50">
        <TransitionChild
          enter="ease-out duration-300"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-200"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-black/30" aria-hidden="true" />
        </TransitionChild>

        <div className="fixed inset-x-0 bottom-0 flex items-end justify-center">
          <TransitionChild
            enter="ease-out duration-300"
            enterFrom="opacity-0 translate-y-full"
            enterTo="opacity-100 translate-y-0"
            leave="ease-in duration-200"
            leaveFrom="opacity-100 translate-y-0"
            leaveTo="opacity-0 translate-y-full"
          >
            <DialogPanel className="w-full max-w-4xl bg-white dark:bg-gray-900 rounded-t-2xl shadow-xl">
              <div className="p-6">
                {/* Header */}
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center space-x-3">
                    <DialogTitle className="text-lg font-semibold text-gray-900 dark:text-white">
                      FFmpeg Command - {streamId}
                    </DialogTitle>
                    {currentCommand?.is_custom && (
                      <span className="px-2 py-1 text-xs font-medium bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-200 rounded">
                        Custom
                      </span>
                    )}
                  </div>
                  <button
                    onClick={onClose}
                    className="p-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition"
                  >
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>

                {/* Encoder Override (only in view mode and if not custom) */}
                {mode === 'view' && !currentCommand?.is_custom && (
                  <div className="mb-4">
                    <label className="flex items-center space-x-2 cursor-pointer mb-2">
                      <input
                        type="checkbox"
                        checked={showEncoderOverride}
                        onChange={(e) => {
                          setShowEncoderOverride(e.target.checked);
                          if (!e.target.checked) {
                            setSelectedEncoder('');
                          }
                        }}
                        className="w-4 h-4 text-blue-600 bg-gray-100 border-gray-300 rounded focus:ring-blue-500 dark:focus:ring-blue-600 dark:ring-offset-gray-800 focus:ring-2 dark:bg-gray-700 dark:border-gray-600"
                      />
                      <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
                        Override encoder
                      </span>
                    </label>
                    
                    {showEncoderOverride && (
                      <select
                        value={selectedEncoder}
                        onChange={(e) => handleEncoderChange(e.target.value)}
                        className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
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
                        <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-blue-500"></div>
                      </div>
                    );
                  }
                  
                  if (!currentCommand) {
                    return (
                      <div className="text-center py-8 text-gray-500 dark:text-gray-400">
                        No command data available
                      </div>
                    );
                  }
                  
                  return (
                  <div className="space-y-4">
                    <div className="relative group">
                      {mode === 'edit' ? (
                        <textarea
                          value={editedCommand}
                          onChange={(e) => setEditedCommand(e.target.value)}
                          className="w-full h-48 p-4 font-mono text-sm text-gray-900 dark:text-gray-100 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg resize-none focus:ring-2 focus:ring-blue-500 focus:border-transparent overflow-auto break-all"
                          placeholder="Enter FFmpeg command..."
                          disabled={saving}
                          spellCheck={false}
                          wrap="soft"
                        />
                      ) : (
                        <>
                          <pre className="w-full h-48 p-4 font-mono text-sm text-gray-900 dark:text-gray-100 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg overflow-auto whitespace-pre-wrap break-all">
                            {currentCommand.command}
                          </pre>
                          <button
                            onClick={copyToClipboard}
                            className="absolute top-2 right-2 p-2 bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300 rounded opacity-0 group-hover:opacity-100 transition"
                            title="Copy to clipboard"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                            </svg>
                          </button>
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
              </div>
            </DialogPanel>
          </TransitionChild>
        </div>
      </Dialog>
    </Transition>
  );
}
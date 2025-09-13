import { useState, useEffect } from 'react';
import { Dialog, DialogPanel, DialogTitle, Transition, TransitionChild } from '@headlessui/react';
import { Button } from './Button';
import { getFFmpegCommand, setFFmpegCommand, clearFFmpegCommand, FFmpegCommandData } from '../lib/api';
import toast from 'react-hot-toast';

interface FFmpegCommandSheetProps {
  isOpen: boolean;
  onClose: () => void;
  streamId: string;
}

export function FFmpegCommandSheet({ isOpen, onClose, streamId }: Readonly<FFmpegCommandSheetProps>) {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [commandData, setCommandData] = useState<FFmpegCommandData | null>(null);
  const [editedCommand, setEditedCommand] = useState('');
  const [isEditing, setIsEditing] = useState(false);

  useEffect(() => {
    if (isOpen && streamId) {
      fetchCommand();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, streamId]);

  const fetchCommand = async () => {
    setLoading(true);
    try {
      const data = await getFFmpegCommand(streamId);
      setCommandData(data);
      setEditedCommand(data.command);
      setIsEditing(false);
    } catch (error) {
      console.error('Failed to fetch FFmpeg command:', error);
      toast.error('Failed to load FFmpeg command');
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    if (!editedCommand.trim()) {
      toast.error('Command cannot be empty');
      return;
    }

    setSaving(true);
    try {
      const data = await setFFmpegCommand(streamId, editedCommand);
      setCommandData(data);
      setIsEditing(false);
      toast.success('FFmpeg command updated successfully');
    } catch (error) {
      console.error('Failed to save FFmpeg command:', error);
      toast.error('Failed to save FFmpeg command');
    } finally {
      setSaving(false);
    }
  };

  const handleRevertToAuto = async () => {
    setSaving(true);
    try {
      await clearFFmpegCommand(streamId);
      await fetchCommand();
      toast.success('Reverted to auto-generated command');
    } catch (error) {
      console.error('Failed to clear custom command:', error);
      toast.error('Failed to revert to auto-generated command');
    } finally {
      setSaving(false);
    }
  };

  const handleCopyToClipboard = async () => {
    if (commandData?.command) {
      try {
        await navigator.clipboard.writeText(commandData.command);
        toast.success('Command copied to clipboard');
      } catch (error) {
        console.error('Failed to copy to clipboard:', error);
        toast.error('Failed to copy to clipboard');
      }
    }
  };

  const handleCancel = () => {
    setEditedCommand(commandData?.command || '');
    setIsEditing(false);
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
                    {commandData?.is_custom && (
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

                {/* Content */}
                {(() => {
                  if (loading) {
                    return (
                      <div className="flex items-center justify-center h-48">
                        <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-blue-500"></div>
                      </div>
                    );
                  }
                  
                  if (!commandData) {
                    return (
                      <div className="text-center py-8 text-gray-500 dark:text-gray-400">
                        No command data available
                      </div>
                    );
                  }
                  
                  return (
                    <div className="space-y-4">
                      <div className="relative group">
                        {isEditing ? (
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
                              {commandData.command}
                            </pre>
                            <button
                              onClick={handleCopyToClipboard}
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
                        {!isEditing ? (
                          <>
                            <Button
                              theme="primary"
                              size="MD"
                              onClick={() => setIsEditing(true)}
                              text="Edit Command"
                            />
                            {commandData.is_custom && (
                              <Button
                                theme="light"
                                size="MD"
                                onClick={handleRevertToAuto}
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
                              onClick={handleSave}
                              disabled={saving}
                              text={saving ? 'Saving...' : 'Save'}
                            />
                            <Button
                              theme="light"
                              size="MD"
                              onClick={handleCancel}
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
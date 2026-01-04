import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { Card } from './Card';
import { Button } from './Button';
import { WebRTCPreview } from './WebRTCPreview';
import { FFmpegCommandSheet } from './FFmpegCommandSheet';
import { StreamData, buildStreamURL, toggleTestMode } from '../lib/api';
import { truncateDeviceId } from '../utils';

interface StreamCardProps {
  stream: StreamData;
  onDelete?: (streamId: string) => void;
  onRefresh?: (streamId: string) => void;
  showVideo?: boolean;
  className?: string;
}

export function StreamCard({ stream, onDelete, onRefresh, showVideo = true, className = '' }: Readonly<StreamCardProps>) {
  const navigate = useNavigate();
  const [isDeleting, setIsDeleting] = useState(false);
  const [isTogglingTestMode, setIsTogglingTestMode] = useState(false);
  const [showFFmpegSheet, setShowFFmpegSheet] = useState(false);
  const [iframeKey, setIframeKey] = useState(0);

  const handleDelete = async () => {
    if (!onDelete || isDeleting) return;
    
    setIsDeleting(true);
    try {
      await onDelete(stream.stream_id);
    } catch (error) {
      console.error('Failed to delete stream:', error);
    } finally {
      setIsDeleting(false);
    }
  };

  const handleEdit = () => {
    navigate(`/streams/${stream.stream_id}/edit`);
  };

  const handleToggleTestMode = async () => {
    setIsTogglingTestMode(true);
    const newTestMode = !stream.test_mode;
    
    try {
      await toggleTestMode(stream.stream_id, newTestMode);
      
      if (newTestMode) {
        toast.success('Test mode enabled');
      } else {
        toast.success('Test mode disabled');
      }
      
      // Force refresh the iframe
      setIframeKey(prev => prev + 1);
      
      if (onRefresh) {
        await onRefresh(stream.stream_id);
      }
    } catch (error) {
      console.error('Failed to toggle test mode:', error);
      toast.error('Failed to toggle test mode');
    } finally {
      setIsTogglingTestMode(false);
    }
  };

  const calculateUptime = (startTime?: string) => {
    if (!startTime) return 'N/A';
    
    try {
      const start = new Date(startTime);
      const now = new Date();
      const uptimeMs = now.getTime() - start.getTime();
      
      if (uptimeMs < 0) return 'N/A';
      
      const seconds = Math.floor(uptimeMs / 1000);
      const hours = Math.floor(seconds / 3600);
      const minutes = Math.floor((seconds % 3600) / 60);
      const remainingSeconds = seconds % 60;
      
      return `${hours}h, ${minutes}m, ${remainingSeconds}s`;
    } catch {
      return 'N/A';
    }
  };

  return (
    <Card className={`h-full ${className}`}>
      <Card.Header className="pb-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white truncate">
            {stream.stream_id}
          </h3>
          <div className="flex items-center space-x-1">
            <div title={stream.custom_ffmpeg_command ? "Custom FFmpeg Command" : "FFmpeg Command"}>
              <Button
                theme={stream.custom_ffmpeg_command ? "primary" : "blank"}
                size="SM"
                onClick={() => setShowFFmpegSheet(true)}
                LeadingIcon={({ className }) => (
                  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
                  </svg>
                )}
              />
            </div>
            
            <div title="Edit Stream">
              <Button
                theme="blank"
                size="SM"
                onClick={handleEdit}
                LeadingIcon={({ className }) => (
                  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                  </svg>
                )}
              />
            </div>
            
            <div title={(() => {
              if (stream.custom_ffmpeg_command) return "Test mode disabled when custom command is set";
              if (stream.test_mode) return "Disable Test Mode";
              return "Enable Test Mode";
            })()}>
              <Button
                theme={stream.test_mode ? "primary" : "blank"}
                size="SM"
                onClick={handleToggleTestMode}
                disabled={isTogglingTestMode || !!stream.custom_ffmpeg_command}
                LeadingIcon={({ className }) => (
                  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z" />
                  </svg>
                )}
              />
            </div>
            
            {onDelete && (
              <div title="Delete Stream">
                <Button
                  theme="danger"
                  size="SM"
                  onClick={handleDelete}
                  disabled={isDeleting}
                  LeadingIcon={({ className }) => (
                    <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                  )}
                />
              </div>
            )}
          </div>
        </div>
      </Card.Header>
      
      <Card.Content className="space-y-4">
        {/* WebRTC Preview Area */}
        {showVideo && (
          <div className="aspect-video bg-gray-100 dark:bg-gray-800 rounded-lg overflow-hidden">
            <WebRTCPreview
              streamId={stream.stream_id}
              className="w-full h-full"
              refreshKey={iframeKey}
            />
          </div>
        )}

        {/* Stream Metadata */}
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Device:</span>
            <span className="text-gray-900 dark:text-white font-medium truncate ml-2" title={stream.device_id}>
              {truncateDeviceId(stream.device_id)}
            </span>
          </div>
          
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Codec:</span>
            <span className="text-gray-900 dark:text-white font-medium uppercase">
              {stream.input_format ? `${stream.input_format}/${stream.codec}` : stream.codec}
            </span>
          </div>
          
          {stream.bitrate && (
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-300">Bitrate:</span>
              <span className="text-gray-900 dark:text-white font-medium">
                {stream.bitrate}
              </span>
            </div>
          )}
          
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Uptime:</span>
            <span className="text-gray-900 dark:text-white font-medium">
              {calculateUptime(stream.start_time)}
            </span>
          </div>
          
          {stream.fps && (
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-300">FPS:</span>
              <span className="text-gray-900 dark:text-white font-medium">
                {stream.fps}
              </span>
            </div>
          )}
          

          {stream.dropped_frames && (
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-300">Dropped Frames:</span>
              <span className="text-gray-900 dark:text-white font-medium">
                {stream.dropped_frames}
              </span>
            </div>
          )}
          
          {stream.processing_speed && (
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-300">Processing Speed:</span>
              <span className="text-gray-900 dark:text-white font-medium">
                {stream.processing_speed}x
              </span>
            </div>
          )}
        </div>

        {/* Stream URLs */}
        {stream.rtsp_url && (
          <div className="space-y-2">
            <h4 className="text-sm font-medium text-gray-900 dark:text-white">Stream URLs:</h4>
            <div className="flex items-center space-x-2">
              <span className="text-xs bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200 px-2 py-1 rounded">
                RTSP
              </span>
              <code className="text-xs text-gray-600 dark:text-gray-300 truncate flex-1">
                {buildStreamURL(stream.rtsp_url, 'rtsp')}
              </code>
            </div>
          </div>
        )}


      </Card.Content>
      
      {/* FFmpeg Command Sheet */}
      <FFmpegCommandSheet 
        isOpen={showFFmpegSheet}
        onClose={() => setShowFFmpegSheet(false)}
        streamId={stream.stream_id}
        {...(onRefresh && { onRefresh })}
      />
    </Card>
  );
}
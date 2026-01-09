import { useState, useCallback } from 'react';
import { Card } from './Card';
import { WebRTCPlayer } from './webrtc';
import { FFmpegCommandSheet } from './FFmpegCommandSheet';
import { StreamCardActions } from './StreamCardActions';
import { StreamMetrics } from './StreamMetrics';
import { buildStreamURL } from '../lib/api';
import { useStreamStore } from '../hooks/useStreamStore';

interface StreamCardProps {
  streamId: string;
  onDelete?: (streamId: string) => void;
  onRefresh?: (streamId: string) => void;
  showVideo?: boolean;
  className?: string;
}

export function StreamCard({ streamId, onDelete, onRefresh, showVideo = true, className = '' }: Readonly<StreamCardProps>) {
  // Subscribe directly to this stream - only re-renders when THIS stream changes
  const stream = useStreamStore((state) => state.streamsById[streamId]);

  const [showFFmpegSheet, setShowFFmpegSheet] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);

  const handleRequestPlayerRefresh = useCallback(() => {
    setRefreshKey(prev => prev + 1);
  }, []);

  const handleShowFFmpegSheet = useCallback(() => {
    setShowFFmpegSheet(true);
  }, []);

  const handleCloseFFmpegSheet = useCallback(() => {
    setShowFFmpegSheet(false);
  }, []);

  // Guard against missing stream (e.g., after deletion)
  if (!stream) return null;

  return (
    <Card className={`h-full ${className}`}>
      <Card.Header className="pb-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white truncate">
            {stream.stream_id}
          </h3>
          <StreamCardActions
            streamId={streamId}
            onDelete={onDelete}
            onRefresh={onRefresh}
            onShowFFmpegSheet={handleShowFFmpegSheet}
            onRequestPlayerRefresh={handleRequestPlayerRefresh}
          />
        </div>
      </Card.Header>
      
      <Card.Content className="space-y-4">
        {/* WebRTC Preview Area */}
        {showVideo && (
          <div className="aspect-video bg-gray-100 dark:bg-gray-800 rounded-lg overflow-hidden">
            <WebRTCPlayer
              key={refreshKey}
              streamId={stream.stream_id}
              className="w-full h-full"
              showStats={true}
            />
          </div>
        )}

        {/* Stream Metadata */}
        <div className="space-y-2 text-sm">
          <div className="flex justify-between gap-2">
            <span className="text-gray-600 dark:text-gray-300 shrink-0">Device:</span>
            <span className="text-gray-900 dark:text-white font-medium font-mono truncate" title={stream.device_id}>
              {stream.device_id}
            </span>
          </div>

          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Codec (in/out):</span>
            <span className="text-gray-900 dark:text-white font-medium font-mono uppercase">
              {stream.input_format ? `${stream.input_format}/${stream.codec}` : stream.codec}
            </span>
          </div>

          {stream.bitrate && (
            <div className="flex justify-between">
              <span className="text-gray-600 dark:text-gray-300">Bitrate:</span>
              <span className="text-gray-900 dark:text-white font-medium font-mono">
                {stream.bitrate}
              </span>
            </div>
          )}

          <StreamMetrics streamId={streamId} />
        </div>

        {/* Stream URLs */}
        <div className="space-y-2">
          <h4 className="text-sm font-medium text-gray-900 dark:text-white">Stream URLs:</h4>
          <div className="flex items-center space-x-2">
            <span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 px-2 py-1 rounded">
              WebRTC
            </span>
            <code className="text-xs text-gray-600 dark:text-gray-300 truncate flex-1">
              {`${window.location.origin}/video?stream=${stream.stream_id}`}
            </code>
          </div>
          {stream.rtsp_url && (
            <div className="flex items-center space-x-2">
              <span className="text-xs bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200 px-2 py-1 rounded">
                RTSP
              </span>
              <code className="text-xs text-gray-600 dark:text-gray-300 truncate flex-1">
                {buildStreamURL(stream.rtsp_url, 'rtsp')}
              </code>
            </div>
          )}
        </div>


      </Card.Content>
      
      {/* FFmpeg Command Sheet */}
      <FFmpegCommandSheet
        isOpen={showFFmpegSheet}
        onClose={handleCloseFFmpegSheet}
        streamId={stream.stream_id}
        {...(onRefresh && { onRefresh })}
      />
    </Card>
  );
}
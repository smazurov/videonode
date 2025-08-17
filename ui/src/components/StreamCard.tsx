import { useState } from 'react';
import { Card } from './Card';
import { Button } from './Button';
import { WebRTCPreview } from './WebRTCPreview';
import { StreamData } from '../lib/api';

interface StreamCardProps {
  stream: StreamData;
  onDelete?: (streamId: string) => void;
  onCapture?: (streamId: string) => void;
  onRefresh?: (streamId: string) => void;
  className?: string;
}

export function StreamCard({ stream, onDelete, onCapture, onRefresh, className = '' }: Readonly<StreamCardProps>) {
  const [isDeleting, setIsDeleting] = useState(false);
  const [isCapturing, setIsCapturing] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);

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

  const handleCapture = async () => {
    if (isCapturing) return;
    
    setIsCapturing(true);
    try {
      if (onCapture) {
        await onCapture(stream.stream_id);
      } else {
        // Direct capture using device path if available
        // Note: We'd need the device path, which isn't in StreamData
        // This is a fallback implementation
        console.log('Capturing screenshot from stream:', stream.stream_id);
      }
    } catch (error) {
      console.error('Failed to capture from stream:', error);
    } finally {
      setIsCapturing(false);
    }
  };

  const handleRefresh = async () => {
    if (!onRefresh || isRefreshing) return;
    
    setIsRefreshing(true);
    try {
      await onRefresh(stream.stream_id);
    } catch (error) {
      console.error('Failed to refresh stream:', error);
    } finally {
      setIsRefreshing(false);
    }
  };

  const formatUptime = (uptimeNs?: number) => {
    if (!uptimeNs) return 'N/A';
    
    const uptimeMs = uptimeNs / 1000000;
    const seconds = Math.floor(uptimeMs / 1000);
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const remainingSeconds = seconds % 60;
    
    if (hours > 0) {
      return `${hours}h ${minutes}m ${remainingSeconds}s`;
    } else if (minutes > 0) {
      return `${minutes}m ${remainingSeconds}s`;
    } else {
      return `${remainingSeconds}s`;
    }
  };

  const formatStartTime = (startTime?: string) => {
    if (!startTime) return 'N/A';
    
    try {
      const date = new Date(startTime);
      return date.toLocaleString();
    } catch {
      return 'Invalid date';
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
            <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
            <span className="text-xs text-green-600 dark:text-green-400">Live</span>
          </div>
        </div>
      </Card.Header>
      
      <Card.Content className="space-y-4">
        {/* WebRTC Preview Area */}
        <div className="aspect-video bg-gray-100 dark:bg-gray-800 rounded-lg overflow-hidden">
          <WebRTCPreview
            streamId={stream.stream_id}
            webrtcUrl={stream.webrtc_url}
            className="w-full h-full"
          />
        </div>

        {/* Stream Metadata */}
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Device:</span>
            <span className="text-gray-900 dark:text-white font-medium truncate ml-2">
              {stream.device_id}
            </span>
          </div>
          
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Codec:</span>
            <span className="text-gray-900 dark:text-white font-medium uppercase">
              {stream.codec}
            </span>
          </div>
          
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Uptime:</span>
            <span className="text-gray-900 dark:text-white font-medium">
              {formatUptime(stream.uptime)}
            </span>
          </div>
          
          <div className="flex justify-between">
            <span className="text-gray-600 dark:text-gray-300">Started:</span>
            <span className="text-gray-900 dark:text-white font-medium">
              {formatStartTime(stream.start_time)}
            </span>
          </div>
        </div>

        {/* Stream URLs */}
        {(stream.webrtc_url || stream.rtsp_url) && (
          <div className="space-y-2">
            <h4 className="text-sm font-medium text-gray-900 dark:text-white">Stream URLs:</h4>
            {stream.webrtc_url && (
              <div className="flex items-center space-x-2">
                <span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 px-2 py-1 rounded">
                  WebRTC
                </span>
                <code className="text-xs text-gray-600 dark:text-gray-300 truncate flex-1">
                  {stream.webrtc_url}
                </code>
              </div>
            )}
            {stream.rtsp_url && (
              <div className="flex items-center space-x-2">
                <span className="text-xs bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200 px-2 py-1 rounded">
                  RTSP
                </span>
                <code className="text-xs text-gray-600 dark:text-gray-300 truncate flex-1">
                  {stream.rtsp_url}
                </code>
              </div>
            )}
          </div>
        )}

        {/* Action Buttons */}
        <div className="flex space-x-2 pt-2">
          <Button
            theme="light"
            size="SM"
            onClick={handleCapture}
            disabled={isCapturing}
            className="flex-1"
            text={isCapturing ? 'Capturing...' : 'Capture'}
          />
          
          {onRefresh && (
            <Button
              theme="light"
              size="SM"
              onClick={handleRefresh}
              disabled={isRefreshing}
              className="flex-1"
              text={isRefreshing ? 'Refreshing...' : 'Refresh'}
            />
          )}
          
          {onDelete && (
            <Button
              theme="danger"
              size="SM"
              onClick={handleDelete}
              disabled={isDeleting}
              className="flex-1"
              text={isDeleting ? 'Deleting...' : 'Delete'}
            />
          )}
        </div>
      </Card.Content>
    </Card>
  );
}
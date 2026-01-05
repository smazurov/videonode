import { useState, useEffect } from 'react';
import { StreamCard } from './StreamCard';
import { StreamData } from '../lib/api';
import { Button } from './Button';
import { Card } from './Card';

const SHOW_VIDEOS_KEY = 'streamGrid.showVideos';

export interface StreamsGridProps {
  streams: StreamData[];
  loading?: boolean;
  error?: string | null;
  onRefresh?: () => void;
  onDeleteStream?: (streamId: string) => void;
  onCreateStream?: () => void;
  className?: string;
}

export function StreamsGrid({
  streams,
  loading = false,
  error = null,
  onRefresh,
  onDeleteStream,
  onCreateStream,
  className = ''
}: Readonly<StreamsGridProps>) {
  const [showVideos, setShowVideos] = useState(() => {
    const stored = localStorage.getItem(SHOW_VIDEOS_KEY);
    return stored !== null ? stored === 'true' : true;
  });

  useEffect(() => {
    localStorage.setItem(SHOW_VIDEOS_KEY, String(showVideos));
  }, [showVideos]);

  const renderGridView = () => (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
      {streams.map((stream) => (
        <StreamCard
          key={stream.stream_id}
          streamId={stream.stream_id}
          showVideo={showVideos}
          {...(onDeleteStream && { onDelete: onDeleteStream })}
          {...(onRefresh && { onRefresh: () => onRefresh() })}
        />
      ))}
    </div>
  );



  const renderEmptyState = () => (
    <Card className="text-center py-12">
      <Card.Content>
        <div className="w-16 h-16 bg-gray-200 dark:bg-gray-700 rounded-full flex items-center justify-center mx-auto mb-4">
          <svg
            className="w-8 h-8 text-gray-400 dark:text-gray-500"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
          No active streams
        </h3>
        <p className="text-gray-600 dark:text-gray-300 mb-6">
          Create your first video stream to get started
        </p>
        {onCreateStream && (
          <Button 
            onClick={onCreateStream}
            theme="primary"
            size="LG"
            LeadingIcon={({ className }) => (
              <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
              </svg>
            )}
            text="Create Stream"
          />
        )}
      </Card.Content>
    </Card>
  );

  const renderLoadingState = () => (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
      {Array.from({ length: 3 }, (_, index) => (
        <Card key={index} className="h-full">
          <Card.Header className="pb-3">
            <div className="flex items-center justify-between">
              <div className="w-24 h-5 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
              <div className="w-12 h-4 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
            </div>
          </Card.Header>
          <Card.Content className="space-y-4">
            <div className="aspect-video bg-gray-200 dark:bg-gray-700 rounded-lg animate-pulse" />
            <div className="space-y-2">
              {Array.from({ length: 4 }, (_, i) => (
                <div key={i} className="flex justify-between">
                  <div className="w-16 h-4 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
                  <div className="w-20 h-4 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
                </div>
              ))}
            </div>
            <div className="flex space-x-2 pt-2">
              <div className="flex-1 h-8 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
              <div className="flex-1 h-8 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
            </div>
          </Card.Content>
        </Card>
      ))}
    </div>
  );

  const renderErrorState = () => (
    <Card className="text-center py-12">
      <Card.Content>
        <div className="w-16 h-16 bg-red-100 dark:bg-red-900 rounded-full flex items-center justify-center mx-auto mb-4">
          <svg
            className="w-8 h-8 text-red-600 dark:text-red-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.651 16.5c-.77.833.192 2.5 1.732 2.5z"
            />
          </svg>
        </div>
        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
          Failed to load streams
        </h3>
        <p className="text-gray-600 dark:text-gray-300 mb-6">
          {error || 'An error occurred while fetching streams'}
        </p>
        {onRefresh && (
          <Button 
            onClick={onRefresh} 
            theme="light"
            size="MD"
            text="Try Again"
          />
        )}
      </Card.Content>
    </Card>
  );

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Header with Controls */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">
            Video Streams
          </h2>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {streams.length} active {streams.length === 1 ? 'stream' : 'streams'}
          </p>
        </div>
        
        <div className="flex items-center space-x-3">
          {/* Show Videos Checkbox */}
          {streams.length > 0 && (
            <label className="flex items-center space-x-2 cursor-pointer">
              <input
                type="checkbox"
                checked={showVideos}
                onChange={(e) => setShowVideos(e.target.checked)}
                className="w-4 h-4 text-blue-600 bg-gray-100 border-gray-300 rounded focus:ring-blue-500 dark:focus:ring-blue-600 dark:ring-offset-gray-800 focus:ring-2 dark:bg-gray-700 dark:border-gray-600"
              />
              <span className="text-sm text-gray-700 dark:text-gray-300">Show Videos</span>
            </label>
          )}
          
          {/* Action Buttons */}
          <div className="flex space-x-2">
            {onRefresh && (
              <Button
                theme="light"
                size="MD"
                onClick={onRefresh}
                disabled={loading}
                text={loading ? 'Refreshing...' : 'Refresh'}
              />
            )}
            
            {onCreateStream && (
              <Button 
                onClick={onCreateStream}
                theme="primary"
                size="MD"
                LeadingIcon={({ className }) => (
                  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                )}
                text="Create Stream"
              />
            )}
          </div>
        </div>
      </div>

      {/* Content */}
      {(() => {
        if (loading) return renderLoadingState();
        if (error) return renderErrorState();
        if (streams.length === 0) return renderEmptyState();
        return renderGridView();
      })()}
    </div>
  );
}
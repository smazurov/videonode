import { useState, useEffect } from 'react';
import {
  ChevronDownIcon,
  ExclamationTriangleIcon,
  PlusIcon,
  VideoCameraIcon,
} from '@heroicons/react/24/outline';
import { Menu, MenuButton, MenuItem, MenuItems } from '@headlessui/react';
import { StreamCard } from './StreamCard';
import { CanvasMembersGroupCard } from './CanvasMembersGroupCard';
import { Button } from './Button';
import { Card } from './Card';
import { Checkbox } from './Checkbox';
import { useStreamStore } from '../hooks/useStreamStore';

const SHOW_VIDEOS_KEY = 'streamGrid.showVideos';

export interface StreamsGridProps {
  streamIds: string[];
  loading?: boolean;
  error?: string | null;
  onRefresh?: () => void;
  onDeleteStream?: (streamId: string) => void;
  onCreateStream?: () => void;
  onCreateCanvas?: () => void;
  className?: string;
}

export function StreamsGrid({
  streamIds,
  loading = false,
  error = null,
  onRefresh,
  onDeleteStream,
  onCreateStream,
  onCreateCanvas,
  className = ''
}: Readonly<StreamsGridProps>) {
  const [showVideos, setShowVideos] = useState(() => {
    const stored = localStorage.getItem(SHOW_VIDEOS_KEY);
    return stored !== null ? stored === 'true' : true;
  });

  useEffect(() => {
    localStorage.setItem(SHOW_VIDEOS_KEY, String(showVideos));
  }, [showVideos]);

  const streamsById = useStreamStore((state) => state.streamsById);

  const renderGridView = () => {
    const memberGroups = new Map<string, string[]>();
    for (const id of streamIds) {
      const owner = streamsById[id]?.owned_by;
      if (!owner) continue;
      const list = memberGroups.get(owner) ?? [];
      list.push(id);
      memberGroups.set(owner, list);
    }

    const renderedGroupOwners = new Set<string>();

    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
        {streamIds.map((streamId) => {
          const owner = streamsById[streamId]?.owned_by;
          const group = owner ? memberGroups.get(owner) ?? [] : [];
          if (owner && group.length >= 2) {
            if (renderedGroupOwners.has(owner)) return null;
            renderedGroupOwners.add(owner);
            return (
              <CanvasMembersGroupCard
                key={`group:${owner}`}
                canvasId={owner}
                streamIds={group}
              />
            );
          }
          return (
            <StreamCard
              key={streamId}
              streamId={streamId}
              showVideo={showVideos}
              {...(onDeleteStream && { onDelete: onDeleteStream })}
              {...(onRefresh && { onRefresh })}
            />
          );
        })}
      </div>
    );
  };



  const renderEmptyState = () => (
    <Card className="text-center py-12">
      <Card.Content>
        <div className="w-16 h-16 bg-surface-muted rounded-full flex items-center justify-center mx-auto mb-4">
          <VideoCameraIcon className="w-8 h-8 text-fg-subtle" />
        </div>
        <h3 className="text-lg font-medium text-fg mb-2">
          No active streams
        </h3>
        <p className="text-fg-muted mb-6">
          Create your first video stream to get started
        </p>
        {onCreateStream && (
          <Button
            onClick={onCreateStream}
            theme="primary"
            size="LG"
            LeadingIcon={PlusIcon}
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
              <div className="w-24 h-5 bg-surface-muted rounded animate-pulse" />
              <div className="w-12 h-4 bg-surface-muted rounded animate-pulse" />
            </div>
          </Card.Header>
          <Card.Content className="space-y-4">
            <div className="aspect-video bg-surface-muted rounded-lg animate-pulse" />
            <div className="space-y-2">
              {Array.from({ length: 4 }, (_, i) => (
                <div key={i} className="flex justify-between">
                  <div className="w-16 h-4 bg-surface-muted rounded animate-pulse" />
                  <div className="w-20 h-4 bg-surface-muted rounded animate-pulse" />
                </div>
              ))}
            </div>
            <div className="flex space-x-2 pt-2">
              <div className="flex-1 h-8 bg-surface-muted rounded animate-pulse" />
              <div className="flex-1 h-8 bg-surface-muted rounded animate-pulse" />
            </div>
          </Card.Content>
        </Card>
      ))}
    </div>
  );

  const renderErrorState = () => (
    <Card className="text-center py-12">
      <Card.Content>
        <div className="w-16 h-16 bg-danger-soft rounded-full flex items-center justify-center mx-auto mb-4">
          <ExclamationTriangleIcon className="w-8 h-8 text-danger-soft-fg" />
        </div>
        <h3 className="text-lg font-medium text-fg mb-2">
          Failed to load streams
        </h3>
        <p className="text-fg-muted mb-6">
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
          <h2 className="text-2xl font-bold text-fg">Video Streams</h2>
          <p className="text-fg-muted mt-1">
            {streamIds.length} active {streamIds.length === 1 ? 'stream' : 'streams'}
          </p>
        </div>

        <div className="flex items-center space-x-3">
          {/* Show Videos Checkbox */}
          {streamIds.length > 0 && (
            <Checkbox
              checked={showVideos}
              onChange={(e) => setShowVideos(e.target.checked)}
              label={<span className="text-fg-muted">Show Videos</span>}
            />
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
              <Menu as="div" className="relative inline-flex">
                <button
                  type="button"
                  onClick={onCreateStream}
                  className="h-10 px-4 inline-flex items-center gap-2 bg-accent hover:bg-accent-hover active:bg-accent-active text-accent-fg font-medium text-sm rounded-l-md border-r border-accent-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-surface"
                >
                  <PlusIcon className="w-5 h-5" />
                  Create Stream
                </button>
                <MenuButton
                  aria-label="More create options"
                  className="h-10 px-2 inline-flex items-center bg-accent hover:bg-accent-hover active:bg-accent-active text-accent-fg rounded-r-md focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-surface"
                >
                  <ChevronDownIcon className="w-4 h-4" />
                </MenuButton>
                <MenuItems
                  anchor="bottom end"
                  className="mt-2 w-64 rounded-md bg-surface-raised border border-border shadow-lg focus:outline-none z-10"
                >
                  {onCreateCanvas && (
                    <MenuItem>
                      {({ focus }) => (
                        <button
                          type="button"
                          onClick={onCreateCanvas}
                          className={`w-full px-4 py-3 text-left flex items-center gap-3 ${
                            focus ? 'bg-surface-muted' : ''
                          }`}
                        >
                          <PlusIcon className="w-5 h-5 text-fg-subtle shrink-0" />
                          <div className="min-w-0">
                            <div className="text-sm font-medium text-fg">
                              Multi-source stream
                            </div>
                            <div className="text-xs text-fg-subtle">
                              Composite 1&ndash;4 sources (formerly &quot;canvas&quot;)
                            </div>
                          </div>
                        </button>
                      )}
                    </MenuItem>
                  )}
                </MenuItems>
              </Menu>
            )}
          </div>
        </div>
      </div>

      {/* Content */}
      {(() => {
        if (loading) return renderLoadingState();
        if (error) return renderErrorState();
        if (streamIds.length === 0) return renderEmptyState();
        return renderGridView();
      })()}
    </div>
  );
}

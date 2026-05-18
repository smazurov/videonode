import { useState, useCallback } from 'react';
import { Card } from './Card';
import { WebRTCPlayer } from './webrtc';
import { FFmpegCommandSheet } from './FFmpegCommandSheet';
import { PerspectiveSheet } from './PerspectiveSheet';
import { StreamLogsSheet } from './StreamLogsSheet';
import { StreamCardActions } from './StreamCardActions';
import { StreamMetrics } from './StreamMetrics';
import { CanvasMemberStreamCard } from './CanvasMemberStreamCard';
import { buildStreamURL } from '../lib/api';
import { Badge } from './Badge';
import { useStreamStore } from '../hooks/useStreamStore';

function formatCodecDisplay(inputFormat: string | undefined, codec: string): string {
  return inputFormat ? `${inputFormat}/${codec}` : codec;
}

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

  type OpenSheet = 'ffmpeg' | 'logs' | 'perspective' | null;
  const [openSheet, setOpenSheet] = useState<OpenSheet>(null);
  const refreshKey = useStreamStore((s) => s.streamRefreshKeys[streamId] ?? 0);
  const bumpStreamRefreshKey = useStreamStore((s) => s.bumpStreamRefreshKey);

  const handleRequestPlayerRefresh = useCallback(() => {
    bumpStreamRefreshKey(streamId);
  }, [bumpStreamRefreshKey, streamId]);

  const handleShowFFmpegSheet = useCallback(() => setOpenSheet('ffmpeg'), []);
  const handleShowLogsSheet = useCallback(() => setOpenSheet('logs'), []);
  const handleShowPerspectiveSheet = useCallback(() => setOpenSheet('perspective'), []);
  const handleCloseSheet = useCallback(() => setOpenSheet(null), []);

  // Guard against missing stream (e.g., after deletion)
  if (!stream) return null;

  // Canvas members get a minimal dedicated card — none of the rtsp/srt URLs,
  // metrics, ffmpeg command viewer, or restart controls apply while owned.
  if (stream.owned_by) {
    return <CanvasMemberStreamCard streamId={streamId} className={className} />;
  }

  const canvas = stream.canvas;
  const sourceStreams = (canvas?.source_streams ?? []).filter((s): s is string => !!s);

  return (
    <Card className={`h-full ${className}`}>
      <Card.Header className="pb-3">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-lg font-semibold text-fg truncate flex items-center gap-2 min-w-0">
            <span className="truncate">{stream.stream_id}</span>
            {stream.owned_by && (
              <Badge tone="rtmp" title={`Device captured by canvas ${stream.owned_by}`}>
                In canvas: {stream.owned_by}
              </Badge>
            )}
          </h3>
          <StreamCardActions
            streamId={streamId}
            onDelete={onDelete}
            onShowFFmpegSheet={handleShowFFmpegSheet}
            onShowLogsSheet={handleShowLogsSheet}
            onShowPerspectiveSheet={handleShowPerspectiveSheet}
            onRequestPlayerRefresh={handleRequestPlayerRefresh}
          />
        </div>
      </Card.Header>

      <Card.Content className="space-y-4">
        {/* WebRTC Preview Area */}
        {showVideo && !stream.owned_by && !(canvas && !stream.enabled) && (
          <div className="aspect-video bg-surface-muted rounded-lg overflow-hidden">
            <WebRTCPlayer
              key={refreshKey}
              streamId={stream.stream_id}
              className="w-full h-full"
              showStats={false}
            />
          </div>
        )}
        {canvas && !stream.enabled && (
          <div className="aspect-video bg-surface-muted rounded-lg overflow-hidden flex items-center justify-center text-fg-muted text-sm">
            Canvas dormant — sources are running individually
          </div>
        )}

        {/* Stream Metadata */}
        <div className="space-y-2 text-sm">
          {canvas ? (
            <>
              <div className="flex justify-between gap-2">
                <span className="text-fg-muted shrink-0">Sources:</span>
                <span className="text-fg font-medium font-mono">
                  {sourceStreams.length} stream{sourceStreams.length === 1 ? '' : 's'}
                </span>
              </div>

              {stream.inputs_enabled && sourceStreams.length > 0 && (
                <div className="flex justify-between gap-2">
                  <span className="text-fg-muted shrink-0">Inputs:</span>
                  <span className="flex items-center gap-1.5 flex-wrap justify-end">
                    {sourceStreams.map((srcID) => {
                      const enabled = stream.inputs_enabled?.[srcID];
                      return (
                        <span
                          key={srcID}
                          className="inline-flex items-center gap-1 text-xs"
                          title={`${srcID}: ${enabled ? 'online' : 'offline'}`}
                        >
                          <span
                            className={`w-2 h-2 rounded-full ${enabled ? 'bg-success' : 'bg-fg-subtle'}`}
                          />
                          <span className="text-fg-muted truncate max-w-[120px]">{srcID}</span>
                        </span>
                      );
                    })}
                  </span>
                </div>
              )}

              <div className="flex justify-between">
                <span className="text-fg-muted">Canvas:</span>
                <span className="text-fg font-medium font-mono">
                  {canvas.width}x{canvas.height} @ {canvas.fps}fps
                </span>
              </div>
            </>
          ) : (
            <div className="flex justify-between gap-2">
              <span className="text-fg-muted shrink-0">Device:</span>
              <span className="text-fg font-medium font-mono truncate" title={stream.device_id}>
                {stream.device_id}
              </span>
            </div>
          )}

          <div className="flex justify-between">
            <span className="text-fg-muted">Codec{canvas ? '' : ' (in/out)'}:</span>
            <span className="text-fg font-medium font-mono uppercase">
              {canvas ? stream.codec : formatCodecDisplay(stream.input_format, stream.codec)}
            </span>
          </div>

          {stream.bitrate && (
            <div className="flex justify-between">
              <span className="text-fg-muted">Bitrate:</span>
              <span className="text-fg font-medium font-mono">{stream.bitrate}</span>
            </div>
          )}

          <StreamMetrics streamId={streamId} />
        </div>

        {/* Stream URLs */}
        <div className="space-y-2">
          <h4 className="text-sm font-medium text-fg">Stream URLs:</h4>
          <div className="flex items-center space-x-2">
            <Badge tone="webrtc" size="md">WebRTC</Badge>
            <code className="text-xs text-fg-muted truncate flex-1">
              {`${window.location.origin}/video?stream=${stream.stream_id}`}
            </code>
          </div>
          {stream.rtsp_url && (
            <div className="flex items-center space-x-2">
              <Badge tone="rtsp" size="md">RTSP</Badge>
              <code className="text-xs text-fg-muted truncate flex-1">
                {buildStreamURL(stream.rtsp_url, 'rtsp')}
              </code>
            </div>
          )}
          {stream.srt_url && (
            <div className="flex items-center space-x-2">
              <Badge tone="srt" size="md">SRT</Badge>
              <code className="text-xs text-fg-muted truncate flex-1">
                {buildStreamURL(stream.srt_url, 'srt')}
              </code>
            </div>
          )}
        </div>
      </Card.Content>

      <FFmpegCommandSheet
        isOpen={openSheet === 'ffmpeg'}
        onClose={handleCloseSheet}
        streamId={stream.stream_id}
        {...(onRefresh && { onRefresh })}
      />
      <StreamLogsSheet
        isOpen={openSheet === 'logs'}
        onClose={handleCloseSheet}
        streamId={stream.stream_id}
      />
      <PerspectiveSheet
        isOpen={openSheet === 'perspective'}
        onClose={handleCloseSheet}
        streamId={stream.stream_id}
        onRequestPlayerRefresh={handleRequestPlayerRefresh}
      />
    </Card>
  );
}

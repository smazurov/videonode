import { useCallback, useState } from 'react';
import { Menu, MenuButton, MenuItems } from '@headlessui/react';
import { AdjustmentsHorizontalIcon, ViewfinderCircleIcon } from '@heroicons/react/24/outline';
import { EllipsisVerticalIcon } from '@heroicons/react/24/solid';
import { Card } from './Card';
import { PerspectiveSheet } from './PerspectiveSheet';
import { InputSpecSheet } from './InputSpecSheet';
import { TestModeMenuItem } from './menu/TestModeMenuItem';
import { MenuRow } from './menu/MenuRow';
import { MENU_DOTS_BUTTON_CLASS } from './menu/menuStyles';
import { Badge } from './Badge';
import { useStreamStore } from '../hooks/useStreamStore';
import { ICON_SIZE } from '../utils';

interface CanvasMemberRowProps {
  streamId: string;
}

export function CanvasMemberRow({ streamId }: Readonly<CanvasMemberRowProps>) {
  const stream = useStreamStore((state) => state.streamsById[streamId]);
  const [showPerspectiveSheet, setShowPerspectiveSheet] = useState(false);
  const [showInputSpecSheet, setShowInputSpecSheet] = useState(false);

  const handleShowPerspectiveSheet = useCallback(() => {
    setShowPerspectiveSheet(true);
  }, []);
  const handleClosePerspectiveSheet = useCallback(() => {
    setShowPerspectiveSheet(false);
  }, []);
  const handleShowInputSpecSheet = useCallback(() => {
    setShowInputSpecSheet(true);
  }, []);
  const handleCloseInputSpecSheet = useCallback(() => {
    setShowInputSpecSheet(false);
  }, []);

  if (!stream) return null;

  const inputSpecParts = [
    stream.input_format,
    stream.resolution,
    stream.framerate ? `${stream.framerate}fps` : undefined,
  ].filter((s): s is string => !!s);
  const inputSpec = inputSpecParts.join(' · ');

  return (
    <div className="space-y-2">
      {/* Header row */}
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-fg truncate min-w-0">
          <span className="truncate">{stream.stream_id}</span>
        </h3>
        <Menu>
          <MenuButton title="More actions" className={MENU_DOTS_BUTTON_CLASS}>
            <EllipsisVerticalIcon className={`${ICON_SIZE.SM} shrink-0`} />
          </MenuButton>
          <MenuItems
            anchor="bottom end"
            className="z-50 mt-1 min-w-[220px] rounded border border-border bg-surface-raised py-1 shadow-lg focus:outline-none"
          >
            <MenuRow icon={AdjustmentsHorizontalIcon} label="Edit Input" onClick={handleShowInputSpecSheet} />
            <MenuRow icon={ViewfinderCircleIcon} label="Perspective Calibration" onClick={handleShowPerspectiveSheet} />
            <TestModeMenuItem streamId={streamId} />
          </MenuItems>
        </Menu>
      </div>

      {/* Device row */}
      <div
        className="text-xs font-mono text-fg-muted truncate"
        title={stream.device_id}
      >
        {stream.device_id || '—'}
      </div>

      {/* Input spec row */}
      {inputSpec && (
        <div className="text-xs font-mono text-fg-muted truncate">{inputSpec}</div>
      )}

      {/* State pills */}
      {(stream.rotation || stream.perspective || stream.test_mode) && (
        <div className="flex items-center gap-1.5 flex-wrap">
          {stream.rotation ? (
            <Badge tone="neutral" size="xs" className="font-mono">
              {stream.rotation}°
            </Badge>
          ) : null}
          {stream.perspective && (
            <button
              type="button"
              onClick={handleShowPerspectiveSheet}
              className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium font-mono shrink-0 bg-canvas-soft text-canvas-soft-fg hover:opacity-90 focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
            >
              perspective
            </button>
          )}
          {stream.test_mode && (
            <Badge tone="webrtc" size="xs" className="font-mono">
              TEST MODE
            </Badge>
          )}
        </div>
      )}

      <PerspectiveSheet
        isOpen={showPerspectiveSheet}
        onClose={handleClosePerspectiveSheet}
        streamId={stream.stream_id}
        onRequestPlayerRefresh={handleClosePerspectiveSheet}
      />
      <InputSpecSheet
        isOpen={showInputSpecSheet}
        onClose={handleCloseInputSpecSheet}
        streamId={stream.stream_id}
      />
    </div>
  );
}

interface CanvasMemberStreamCardProps {
  streamId: string;
  className?: string;
}

export function CanvasMemberStreamCard({
  streamId,
  className = '',
}: Readonly<CanvasMemberStreamCardProps>) {
  const ownedBy = useStreamStore((state) => state.streamsById[streamId]?.owned_by);

  if (!ownedBy) return null;

  return (
    <Card className={`h-full ${className}`}>
      <Card.Content className="space-y-3 py-3">
        <div className="flex items-center">
          <Badge tone="rtmp" size="xs" title={`Device captured by canvas ${ownedBy}`}>
            In canvas: {ownedBy}
          </Badge>
        </div>
        <CanvasMemberRow streamId={streamId} />
      </Card.Content>
    </Card>
  );
}

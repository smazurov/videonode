import { useState, useCallback, useRef, useEffect } from 'react';
import toast from 'react-hot-toast';
import { Button } from './Button';
import { BottomSheet } from './BottomSheet';
import { api } from '../lib/api';
import { useStreamStore } from '../hooks/useStreamStore';
import { PerspectiveAdjust } from './PerspectiveAdjust';
import { PerspectivePreview } from './PerspectivePreview';

interface PerspectiveSheetProps {
  isOpen: boolean;
  onClose: () => void;
  streamId: string;
  onRequestPlayerRefresh: () => void;
}

type Corner = [number, number];
type Tab = 'adjust' | 'preview';

const STREAM_ENDPOINT = "/api/streams/{stream_id}" as const;

export function PerspectiveSheet({ isOpen, onClose, streamId, onRequestPlayerRefresh }: Readonly<PerspectiveSheetProps>) {
  const hasPerspective = useStreamStore((state) => !!state.streamsById[streamId]?.perspective);
  const initialCorners = useStreamStore((state) => state.streamsById[streamId]?.perspective?.corners as Corner[] | undefined);
  const resolution = useStreamStore((state) => state.streamsById[streamId]?.resolution);

  // Parse input resolution (e.g. "1920x1080" → [1920, 1080])
  const [inputWidth, inputHeight] = (resolution?.split('x').map(Number) ?? [1920, 1080]) as [number, number];

  const [corners, setCorners] = useState<Corner[]>([]);
  const [sorted, setSorted] = useState(false);
  const [saving, setSaving] = useState(false);
  const [activeTab, setActiveTab] = useState<Tab>('adjust');
  const initializedRef = useRef(false);

  // Load corners from config on open
  useEffect(() => {
    if (isOpen && !initializedRef.current) {
      initializedRef.current = true;
      if (initialCorners?.length === 4) {
        setCorners([...initialCorners]);
        setSorted(true);
      } else {
        setCorners([]);
        setSorted(false);
      }
      setActiveTab('adjust');
    }
    if (!isOpen) {
      initializedRef.current = false;
    }
  }, [isOpen, initialCorners]);

  // Force back to Adjust if perspective is cleared
  useEffect(() => {
    if (!hasPerspective && activeTab === 'preview') {
      setActiveTab('adjust');
    }
  }, [hasPerspective, activeTab]);

  const handleCornersChange = useCallback((newCorners: Corner[], isSorted: boolean) => {
    setCorners(newCorners);
    setSorted(isSorted);
  }, []);

  const handleApply = useCallback(async () => {
    setSaving(true);
    try {
      const body = corners.length === 4
        ? { perspective: { corners: corners as [Corner, Corner, Corner, Corner] } }
        : { perspective: null };
      const { error } = await api.PATCH(STREAM_ENDPOINT, {
        params: { path: { stream_id: streamId } },
        body,
      });
      if (error) throw new Error(error.detail ?? 'Failed to apply');
      toast.success(corners.length === 4 ? 'Perspective applied' : 'Perspective cleared');
      onRequestPlayerRefresh();
    } catch (error) {
      toast.error('Failed to apply perspective');
      console.error(error);
    } finally {
      setSaving(false);
    }
  }, [corners, streamId, onRequestPlayerRefresh]);

  const handleClear = useCallback(() => {
    setCorners([]);
    setSorted(false);
  }, []);

  const handleClose = useCallback(() => {
    setCorners([]);
    setSorted(false);
    setActiveTab('adjust');
    onRequestPlayerRefresh();
    onClose();
  }, [onRequestPlayerRefresh, onClose]);

  const tabClass = (tab: Tab, enabled: boolean) => {
    const base = 'px-4 py-2 text-sm font-medium rounded-t-lg transition';
    if (!enabled) return `${base} text-fg-subtle cursor-not-allowed`;
    if (activeTab === tab) return `${base} text-accent border-b-2 border-accent`;
    return `${base} text-fg-muted hover:text-fg`;
  };

  return (
    <BottomSheet
      open={isOpen}
      onClose={handleClose}
      title={`Perspective Calibration - ${streamId}`}
      maxWidth="5xl"
      maxHeight="max-h-[90vh]"
      contentClassName="overflow-y-auto"
    >
      <>
                {/* Tab bar */}
                <div className="flex space-x-1 border-b border-border mb-4">
                  <button className={tabClass('adjust', true)} onClick={() => setActiveTab('adjust')}>
                    Adjust
                  </button>
                  <button
                    className={tabClass('preview', hasPerspective)}
                    onClick={() => hasPerspective && setActiveTab('preview')}
                    disabled={!hasPerspective}
                  >
                    Preview
                  </button>
                </div>

                {/* Tab content */}
                {activeTab === 'adjust' && (
                  <>
                    <p className="text-sm text-fg-subtle mb-4">
                      {corners.length < 4
                        ? `Click 4 points on the image to define the region. (${corners.length}/4)`
                        : 'Drag corners to adjust. Click Apply to save.'}
                    </p>
                    <PerspectiveAdjust
                      streamId={streamId}
                      corners={corners}
                      sorted={sorted}
                      onCornersChange={handleCornersChange}
                      inputWidth={inputWidth}
                      inputHeight={inputHeight}
                    />
                  </>
                )}

                {activeTab === 'preview' && (
                  <PerspectivePreview streamId={streamId} />
                )}

                {/* Buttons */}
                <div className="flex items-center space-x-2 mt-4">
                  {activeTab === 'adjust' && (
                    <>
                      {corners.length > 0 && (
                        <Button theme="light" size="SM" text="Clear" onClick={handleClear} disabled={saving} />
                      )}
                      {corners.length === 4 && (
                        <Button theme="primary" size="SM"
                          text={saving ? 'Applying...' : 'Apply'}
                          onClick={handleApply} disabled={saving} />
                      )}
                      {corners.length === 0 && hasPerspective && (
                        <Button theme="danger" size="SM"
                          text={saving ? 'Removing...' : 'Remove Perspective'}
                          onClick={handleApply} disabled={saving} />
                      )}
                    </>
                  )}
                  <Button theme="light" size="SM" text="Close" onClick={handleClose} />
                </div>
      </>
    </BottomSheet>
  );
}

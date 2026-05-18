import { FormEvent } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { AdvancedOptions } from '../StreamCreation/AdvancedOptions';
import { CanvasPreview } from '../CanvasPreview';
import { useCanvasForm, CANVAS_PRESETS, CanvasPreset } from '../../hooks/useCanvasForm';
import type { components } from '../../lib/api.generated';

type StreamData = components['schemas']['StreamData'];

const selectClasses =
  'block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent';

interface CanvasFormProps {
  initialData?: StreamData;
  onSuccess: () => Promise<void>;
  onCancel?: () => void;
  className?: string;
}

export function CanvasForm({
  initialData,
  onSuccess,
  onCancel,
  className = '',
}: Readonly<CanvasFormProps>) {
  const form = useCanvasForm(initialData);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const success = await form.submit();
    if (success) await onSuccess();
  };

  const selectedSourceData = form.sourceIds
    .map((id) => form.availableSources.find((s) => s.stream_id === id))
    .filter((s): s is NonNullable<typeof s> => !!s);
  const unselectedSources = form.availableSources.filter(
    (s) => !form.sourceIds.includes(s.stream_id),
  );

  const canAddMore = form.sourceIds.length < 4;
  const defaultLabel = form.mode === 'edit' ? 'Save Changes' : 'Create Canvas';
  const submitLabel = form.saving ? 'Saving...' : defaultLabel;

  const activeLayout = form.layoutName || form.chosenLayout;

  return (
    <Card className={className}>
      <Card.Content>
        <form onSubmit={handleSubmit} className="space-y-6">
          {/* Header strip: identity & canvas-wide settings */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div>
              <label className="block text-sm font-medium text-fg mb-2">
                Stream ID {form.mode === 'create' && <span className="text-danger">*</span>}
              </label>
              <input
                type="text"
                value={form.streamId}
                onChange={(e) => form.setStreamId(e.target.value)}
                placeholder="my-canvas-001"
                required={form.mode === 'create'}
                disabled={form.saving}
                readOnly={form.mode === 'edit'}
                tabIndex={form.mode === 'edit' ? -1 : undefined}
                aria-readonly={form.mode === 'edit' || undefined}
                className={
                  form.mode === 'edit'
                    ? `${selectClasses} font-mono opacity-60 cursor-not-allowed select-all`
                    : `${selectClasses} font-mono`
                }
              />
              {form.errors.streamId && (
                <p className="mt-1 text-sm text-danger-soft-fg">{form.errors.streamId}</p>
              )}
            </div>
            <div>
              <label className="block text-sm font-medium text-fg mb-2">
                Canvas Size <span className="text-danger">*</span>
              </label>
              <div className="flex gap-2">
                {(Object.keys(CANVAS_PRESETS) as CanvasPreset[]).map((key) => (
                  <button
                    key={key}
                    type="button"
                    onClick={() => form.setPreset(key)}
                    disabled={form.saving}
                    className={`flex-1 block px-3 py-2 border rounded-md shadow-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring ${
                      form.preset === key
                        ? 'border-accent bg-accent-soft text-accent-soft-fg'
                        : 'border-border bg-surface text-fg hover:border-border-strong'
                    }`}
                  >
                    {CANVAS_PRESETS[key].label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <label className="block text-sm font-medium text-fg mb-2">
                FPS <span className="text-danger">*</span>
              </label>
              <select
                value={form.fps}
                onChange={(e) => form.setFps(e.target.value)}
                className={selectClasses}
                disabled={form.saving}
                required
              >
                {form.fpsOptions.map((fps) => (
                  <option key={fps} value={fps}>
                    {fps} FPS
                  </option>
                ))}
              </select>
              {form.errors.fps && (
                <p className="mt-1 text-sm text-danger-soft-fg">{form.errors.fps}</p>
              )}
            </div>
            <div>
              <label className="block text-sm font-medium text-fg mb-2">
                Background Color
              </label>
              <input
                type="text"
                value={form.keyColor}
                onChange={(e) => form.setKeyColor(e.target.value)}
                placeholder="0x000000"
                disabled={form.saving}
                className={selectClasses}
              />
            </div>
          </div>

          {/* Body: sources/audio/advanced on left, preview + encode on right */}
          <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_44rem] gap-6">
            <div className="space-y-6 min-w-0">
              {/* Source streams */}
              <div>
            <label className="block text-sm font-medium text-fg mb-2">
              Source Streams <span className="text-danger">*</span>{' '}
              <span className="text-xs text-fg-subtle">({form.sourceIds.length}/4)</span>
            </label>

            {/* Selected sources (ordered) */}
            {selectedSourceData.length > 0 && (
              <div className="space-y-2 mb-3">
                {selectedSourceData.map((source, index) => (
                  <div
                    key={source.stream_id}
                    className="flex items-center gap-2 p-3 border border-border rounded-md bg-surface-muted"
                  >
                    <span className="text-xs font-mono text-fg-subtle w-6">{index + 1}.</span>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-fg truncate">
                        {source.stream_id}
                      </div>
                      <div className="text-xs text-fg-subtle flex items-center gap-2">
                        <span>
                          {source.resolution || '—'} @ {source.framerate || '—'} fps
                        </span>
                        {source.perspective && (
                          <span className="px-1.5 py-0.5 rounded bg-canvas-soft text-canvas-soft-fg">
                            perspective
                          </span>
                        )}
                        {source.vision && (
                          <span className="px-1.5 py-0.5 rounded bg-success/15 text-success">
                            vision
                          </span>
                        )}
                      </div>
                    </div>
                    <div className="flex items-center gap-1">
                      <select
                        value={form.rotationOverrides[index] == null ? 'inherit' : String(form.rotationOverrides[index])}
                        onChange={(e) => {
                          const v = e.target.value;
                          if (v === 'inherit') {
                            form.setRotationOverride(index, null);
                          } else {
                            form.setRotationOverride(index, parseInt(v, 10) as 0 | 90 | 180 | 270);
                          }
                        }}
                        disabled={form.saving}
                        className="text-xs px-2 py-1 border border-border rounded bg-surface text-fg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
                        aria-label="Rotation override"
                        title={`Inherit: ${source.rotation ?? 0}°`}
                      >
                        <option value="inherit">inherit ({source.rotation ?? 0}°)</option>
                        <option value="0">0°</option>
                        <option value="90">90°</option>
                        <option value="180">180°</option>
                        <option value="270">270°</option>
                      </select>
                      <button
                        type="button"
                        onClick={() => form.moveSource(index, -1)}
                        disabled={form.saving || index === 0}
                        className="p-1 text-fg-subtle hover:text-fg disabled:opacity-30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring rounded"
                        aria-label="Move up"
                      >
                        ↑
                      </button>
                      <button
                        type="button"
                        onClick={() => form.moveSource(index, 1)}
                        disabled={form.saving || index === selectedSourceData.length - 1}
                        className="p-1 text-fg-subtle hover:text-fg disabled:opacity-30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring rounded"
                        aria-label="Move down"
                      >
                        ↓
                      </button>
                      <button
                        type="button"
                        onClick={() => form.removeSource(source.stream_id)}
                        disabled={form.saving}
                        className="p-1 text-danger hover:text-danger-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring rounded"
                        aria-label="Remove"
                      >
                        ✕
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {/* Add source dropdown */}
            {canAddMore && unselectedSources.length > 0 && (
              <select
                className={selectClasses}
                value=""
                onChange={(e) => {
                  if (e.target.value) form.addSource(e.target.value);
                }}
                disabled={form.saving}
              >
                <option value="">Add source stream...</option>
                {unselectedSources.map((s) => (
                  <option key={s.stream_id} value={s.stream_id}>
                    {s.stream_id} — {s.resolution || 'auto'}
                  </option>
                ))}
              </select>
            )}

            {form.errors.sources && (
              <p className="mt-1 text-sm text-danger-soft-fg">{form.errors.sources}</p>
            )}
            {form.availableSources.length === 0 && (
              <p className="mt-2 text-sm text-fg-subtle">
                No individual streams available. Create individual streams first.
              </p>
            )}
          </div>

          {/* Audio */}
          <div>
            <label className="block text-sm font-medium text-fg mb-2">
              Audio Devices
            </label>
            <div className="space-y-2">
              {form.audioDevices.map((dev, i) => (
                <div key={i} className="flex gap-2">
                  <input
                    type="text"
                    value={dev}
                    onChange={(e) => form.updateAudioDevice(i, e.target.value)}
                    placeholder="hw:4,0"
                    className={selectClasses}
                    disabled={form.saving}
                  />
                  <Button
                    type="button"
                    theme="light"
                    size="MD"
                    onClick={() => form.removeAudioDevice(i)}
                    disabled={form.saving}
                    text="Remove"
                  />
                </div>
              ))}
              <Button
                type="button"
                theme="light"
                size="MD"
                onClick={form.addAudioDevice}
                disabled={form.saving}
                text="Add audio device"
              />
            </div>
            <p className="mt-1 text-xs text-fg-subtle">
              One output audio track per device. In the WebRTC preview only the first device plays;
              others arrive muted.
            </p>
          </div>

              {/* Advanced options */}
              <AdvancedOptions
                selectedOptions={form.options}
                onOptionsChange={form.setOptions}
                disabled={form.saving}
              />
            </div>

            {/* Right column: preview + encode settings (sticky on lg+) */}
            <div className="lg:sticky lg:top-4 self-start space-y-4 min-w-0">
              <div>
                <label className="block text-sm font-medium text-fg mb-2">
                  Layout Preview
                </label>
                <CanvasPreview
                  canvasW={form.canvasDimensions.width}
                  canvasH={form.canvasDimensions.height}
                  layout={form.layout}
                  loading={form.layoutLoading}
                  onCycle={form.cycleLayout}
                  chosenLayout={form.chosenLayout}
                  availableCount={form.availableLayouts.length}
                  hideCaption
                />
                <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-fg-subtle">
                  <span>
                    {form.canvasDimensions.width}×{form.canvasDimensions.height} ·{' '}
                    {form.sourceIds.length} source{form.sourceIds.length === 1 ? '' : 's'}
                  </span>
                  {form.availableLayouts.length > 1 && (
                    <div className="flex flex-wrap gap-1.5">
                      {form.availableLayouts.map((name) => {
                        const active = activeLayout === name;
                        return (
                          <button
                            key={name}
                            type="button"
                            onClick={() => form.setLayoutName(name)}
                            disabled={form.saving}
                            className={`px-2 py-0.5 text-xs rounded border font-mono transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring ${
                              active
                                ? 'border-accent bg-accent-soft text-accent-soft-fg'
                                : 'border-border text-fg-muted hover:border-border-strong'
                            }`}
                          >
                            {name}
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-fg mb-2">
                  Codec <span className="text-danger">*</span>
                </label>
                <select
                  value={form.codec}
                  onChange={(e) => form.setCodec(e.target.value as typeof form.codec)}
                  className={selectClasses}
                  disabled={form.saving}
                  required
                >
                  <option value="h264">H.264</option>
                  <option value="h265">H.265 (HEVC)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-fg mb-2">
                  Bitrate
                </label>
                <div className="relative">
                  <input
                    type="number"
                    value={form.bitrate}
                    onChange={(e) => {
                      const n = parseFloat(e.target.value);
                      if (!isNaN(n)) form.setBitrate(n);
                    }}
                    step="0.5"
                    min="0.5"
                    max="100"
                    className="block w-full pl-3 pr-16 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent"
                    disabled={form.saving}
                  />
                  <div className="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
                    <span className="text-fg-subtle sm:text-sm">Mbps</span>
                  </div>
                </div>
                {form.errors.bitrate && (
                  <p className="mt-1 text-sm text-danger-soft-fg">{form.errors.bitrate}</p>
                )}
              </div>
            </div>
          </div>

          {/* Error message */}
          {form.error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft">
              <p className="text-sm text-danger-soft-fg">{form.error}</p>
            </div>
          )}

          {/* Action buttons */}
          <div className="flex justify-end space-x-3 pt-4 border-t border-border">
            {onCancel && (
              <Button
                type="button"
                theme="light"
                size="MD"
                onClick={onCancel}
                disabled={form.saving}
                text="Cancel"
              />
            )}
            <Button
              type="submit"
              theme="primary"
              size="MD"
              disabled={form.saving || !form.isValid}
              text={submitLabel}
            />
          </div>
        </form>
      </Card.Content>
    </Card>
  );
}

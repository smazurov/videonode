import { FormEvent } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { InputField } from '../InputField';
import { Select } from '../Select';
import { ReadOnlyField } from '../ReadOnlyField';
import { AdvancedOptions } from '../StreamCreation/AdvancedOptions';
import { DeviceInputCapabilities } from '../DeviceInputCapabilities';
import { LegacyCanvasPreview } from '../LegacyCanvasPreview';
import { useDeviceStore } from '../../hooks/useDeviceStore';
import { useStreamForm, CANVAS_PRESETS, CanvasPreset } from '../../hooks/useStreamForm';
import type { components } from '../../lib/api.generated';

type StreamData = components['schemas']['StreamData'];

interface StreamFormProps {
  initialData?: StreamData;
  onSuccess: () => Promise<void>;
  onCancel?: () => void;
  className?: string;
}

const selectClasses =
  'block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent';

export function StreamForm({
  initialData,
  onSuccess,
  onCancel,
  className = '',
}: Readonly<StreamFormProps>) {
  const form = useStreamForm(initialData);
  const devices = useDeviceStore((s) => s.devices);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const success = await form.submit();
    if (success) await onSuccess();
  };

  const deviceDisplayName = (() => {
    const device = devices.find((d) => d.device_id === form.deviceId);
    return device ? `${device.device_name} (${device.device_path})` : form.deviceId;
  })();

  const modeLabel = form.mode === 'edit' ? 'Save Changes' : 'Create Stream';
  const submitLabel = form.saving ? 'Saving...' : modeLabel;

  const streamIdErrorProps = form.errors.streamId ? { error: form.errors.streamId } : {};
  const deviceIdErrorProps = form.errors.deviceId ? { error: form.errors.deviceId } : {};
  const codecErrorProps = form.errors.codec ? { error: form.errors.codec } : {};

  const selectedSourceData = form.sourceIds
    .map((id) => form.allSources.find((s) => s.stream_id === id))
    .filter((s): s is NonNullable<typeof s> => !!s);
  const unselectedSources = form.availableSources;
  const canAddMore = form.sourceIds.length < 4;
  const activeLayout = form.layoutName || form.chosenLayout;

  return (
    <Card className={className}>
      <Card.Content>
        <form onSubmit={handleSubmit} className="space-y-6">
          {/* Stream ID */}
          {form.mode === 'edit' ? (
            <ReadOnlyField label="Stream ID" value={form.streamId} mono />
          ) : (
            <InputField
              label="Stream ID"
              type="text"
              value={form.streamId}
              onChange={(e) => form.setStreamId(e.target.value)}
              placeholder="my-stream-001"
              required
              disabled={form.saving}
              {...streamIdErrorProps}
            />
          )}

          {/* SINGLE-SOURCE: video device + capabilities + rotation */}
          {!form.isMulti && (
            <>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {form.mode === 'edit' ? (
                  <ReadOnlyField label="Video Device" value={deviceDisplayName} />
                ) : (
                  <Select
                    label="Video Device"
                    required
                    value={form.deviceId}
                    onChange={(e) => form.selectDevice(e.target.value)}
                    disabled={form.saving || devices.length === 0}
                    {...deviceIdErrorProps}
                  >
                    <option value="">Select device...</option>
                    {devices.map((device) => (
                      <option key={device.device_id} value={device.device_id}>
                        {device.device_name} ({device.device_path})
                      </option>
                    ))}
                  </Select>
                )}

                <Select
                  label="Rotation"
                  value={form.rotation}
                  onChange={(e) => form.setRotation(parseInt(e.target.value, 10))}
                  disabled={form.saving}
                >
                  <option value={0}>0°</option>
                  <option value={90}>90° CW</option>
                  <option value={180}>180°</option>
                  <option value={270}>270° CW</option>
                </Select>
              </div>

              {(form.mode === 'edit' || form.deviceId) && (
                <DeviceInputCapabilities
                  form={form.inputForm}
                  mode={form.mode}
                  disabled={form.saving}
                  errors={form.errors}
                />
              )}
            </>
          )}

          {/* MULTI-SOURCE: source list + layout preview + canvas geometry */}
          {form.isMulti && (
            <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_44rem] gap-6">
              <div className="space-y-6 min-w-0">
                <div>
                  <label className="block text-sm font-medium text-fg mb-2">
                    Video Sources <span className="text-danger">*</span>{' '}
                    <span className="text-xs text-fg-subtle">({form.sourceIds.length}/4)</span>
                  </label>
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
                            <div className="text-xs text-fg-subtle">
                              {source.resolution || '—'} @ {source.framerate || '—'} fps
                            </div>
                          </div>
                          <div className="flex items-center gap-1">
                            <select
                              value={
                                form.rotationOverrides[index] == null
                                  ? 'inherit'
                                  : String(form.rotationOverrides[index])
                              }
                              onChange={(e) => {
                                const v = e.target.value;
                                form.setRotationOverride(
                                  index,
                                  v === 'inherit' ? null : (parseInt(v, 10) as 0 | 90 | 180 | 270),
                                );
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
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
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
                      {form.fpsOptions.map((f) => (
                        <option key={f} value={f}>
                          {f} FPS
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
              </div>

              <div className="lg:sticky lg:top-4 self-start min-w-0">
                <label className="block text-sm font-medium text-fg mb-2">Layout Preview</label>
                <LegacyCanvasPreview
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
            </div>
          )}

          {/* Audio tracks (always shown — single or multi). Single-source
              streams support at most one track; the daemon's wire format
              only carries one audio_device. Multi-track requires a
              multi-source canvas. */}
          <div>
            <label className="block text-sm font-medium text-fg mb-2">
              Audio Tracks{' '}
              <span className="text-xs text-fg-subtle">
                ({form.audioTracks.length}/{form.isMulti ? 4 : 1})
              </span>
            </label>
            <p className="mb-2 text-xs text-fg-subtle">
              {form.isMulti
                ? 'Each track is a separate audio stream in the published output. RTSP/SRT/MPEG-TS carry multi-track audio.'
                : 'Single-source streams support at most one audio track. Add a video source to enable multi-track audio.'}
            </p>
            <div className="space-y-2">
              {form.audioTracks.map((dev, i) => (
                <div key={i} className="flex gap-2">
                  <input
                    type="text"
                    value={dev}
                    onChange={(e) => form.updateAudioTrack(i, e.target.value)}
                    placeholder="hw:4,0"
                    className={selectClasses}
                    disabled={form.saving}
                  />
                  <Button
                    type="button"
                    theme="light"
                    size="MD"
                    onClick={() => form.removeAudioTrack(i)}
                    disabled={form.saving}
                    text="Remove"
                  />
                </div>
              ))}
              {form.audioTracks.length < (form.isMulti ? 4 : 1) && (
                <Button
                  type="button"
                  theme="light"
                  size="MD"
                  onClick={form.addAudioTrack}
                  disabled={form.saving}
                  text="Add audio track"
                />
              )}
            </div>
          </div>

          {/* Encoder (codec + bitrate) — always */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Select
              label="Codec"
              required
              value={form.codec}
              onChange={(e) => form.setCodec(e.target.value as typeof form.codec)}
              disabled={form.saving}
              {...codecErrorProps}
            >
              <option value="h264">H.264</option>
              <option value="h265">H.265 (HEVC)</option>
            </Select>
            <div>
              <label className="block text-sm font-medium text-fg mb-2">Bitrate</label>
              <div className="relative">
                <input
                  type="number"
                  value={form.bitrate || 2}
                  onChange={(e) => {
                    const mbps = parseFloat(e.target.value);
                    if (!isNaN(mbps) && mbps > 0) form.setBitrate(mbps);
                    else if (e.target.value === '') form.setBitrate(2);
                  }}
                  placeholder="2.0"
                  step="0.1"
                  min="0.1"
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

          {/* Add source affordance — single mode, when standalone sources exist (and we're in edit, or in create after the device is picked). */}
          {!form.isMulti && form.availableSources.length > 0 && (
            <div className="flex flex-wrap items-center gap-3 text-sm text-fg-subtle">
              <span>Compose with another existing stream?</span>
              <select
                className={selectClasses + ' max-w-xs'}
                value=""
                onChange={(e) => {
                  if (e.target.value) form.addSource(e.target.value);
                }}
                disabled={form.saving}
              >
                <option value="">+ Add video source...</option>
                {form.availableSources.map((s) => (
                  <option key={s.stream_id} value={s.stream_id}>
                    {s.stream_id} — {s.resolution || 'auto'}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Advanced ffmpeg options — always */}
          <AdvancedOptions
            selectedOptions={form.options}
            onOptionsChange={form.setOptions}
            disabled={form.saving}
            className="mt-4"
          />

          {form.error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft">
              <p className="text-sm text-danger-soft-fg">{form.error}</p>
            </div>
          )}

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
              disabled={form.saving || !form.isValid || (form.mode === 'edit' && !form.isDirty)}
              text={submitLabel}
            />
          </div>
        </form>
      </Card.Content>
    </Card>
  );
}

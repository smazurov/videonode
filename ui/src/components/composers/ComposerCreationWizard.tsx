import { useMemo, useState } from 'react';
import { Button } from '../Button';
import { InputField } from '../InputField';
import { Select } from '../Select';
import { Checkbox } from '../Checkbox';
import Fieldset from '../Fieldset';
import {
  useComposerForm,
  CANVAS_PRESETS,
  type CanvasPresetKey,
  type WizardStep,
  type ComposerCanvasDims,
  type ComposerLayoutSlot,
} from '../../hooks/useComposerForm';
import { cn } from '../../utils';

// Minimal Source shape consumed by the wizard. U2's useSourceStore will
// expose richer data; this is the contract the wizard cares about.
export interface ComposerWizardSource {
  id: string;
  device?: string | null;
  test_mode?: boolean;
}

interface ComposerCreationWizardProps {
  readonly sources: ComposerWizardSource[];
  readonly existingComposerIds?: string[];
  readonly sourcesLoading?: boolean;
  readonly onSuccess: () => void;
  readonly onCancel: () => void;
}

const STEPS: { key: WizardStep; label: string }[] = [
  { key: 'identity', label: 'Identity & Canvas' },
  { key: 'inputs', label: 'Pick Inputs' },
  { key: 'layout', label: 'Initial Layout' },
];

export function ComposerCreationWizard({
  sources,
  existingComposerIds = [],
  sourcesLoading = false,
  onSuccess,
  onCancel,
}: ComposerCreationWizardProps) {
  const form = useComposerForm({ existingIds: existingComposerIds });

  const stepIdx = STEPS.findIndex((s) => s.key === form.step);
  const isLast = stepIdx === STEPS.length - 1;

  const handleNextOrSubmit = async () => {
    if (isLast) {
      const ok = await form.submit();
      if (ok) onSuccess();
      return;
    }
    if (form.stepValid[form.step]) form.goNext();
  };

  return (
    <div className="space-y-6">
      <Stepper currentStep={form.step} stepValid={form.stepValid} />

      {form.step === 'identity' && (
        <IdentityStep
          composerId={form.composerId}
          setComposerId={form.setComposerId}
          preset={form.preset}
          setPreset={form.setPreset}
          customW={form.customW}
          setCustomW={form.setCustomW}
          customH={form.customH}
          setCustomH={form.setCustomH}
          fps={form.fps}
          setFps={form.setFps}
          errors={form.errors}
        />
      )}

      {form.step === 'inputs' && (
        <InputsStep
          sources={sources}
          sourcesLoading={sourcesLoading}
          selectedSourceIds={form.selectedSourceIds}
          toggleSource={form.toggleSource}
          {...(form.errors.inputs ? { error: form.errors.inputs } : {})}
        />
      )}

      {form.step === 'layout' && (
        <LayoutStep canvas={form.canvas} layout={form.layout} />
      )}

      {form.error && (
        <div className="rounded-sm border border-danger bg-danger-soft px-4 py-3 text-sm text-danger-soft-fg">
          {form.error}
        </div>
      )}

      <div className="flex items-center justify-between">
        <Button
          theme="light"
          size="MD"
          onClick={onCancel}
          text="Cancel"
          disabled={form.saving}
        />
        <div className="flex items-center gap-2">
          {stepIdx > 0 && (
            <Button
              theme="light"
              size="MD"
              onClick={form.goBack}
              text="Back"
              disabled={form.saving}
            />
          )}
          <Button
            theme="primary"
            size="MD"
            onClick={handleNextOrSubmit}
            text={isLast ? 'Create' : 'Next'}
            disabled={!form.stepValid[form.step] || (isLast && !form.isValid)}
            loading={form.saving}
          />
        </div>
      </div>
    </div>
  );
}

function Stepper({
  currentStep,
  stepValid,
}: Readonly<{
  currentStep: WizardStep;
  stepValid: Record<WizardStep, boolean>;
}>) {
  return (
    <ol className="flex items-center gap-2" aria-label="Wizard steps">
      {STEPS.map((s, i) => {
        const isCurrent = currentStep === s.key;
        const isDone =
          STEPS.findIndex((x) => x.key === currentStep) > i && stepValid[s.key];
        return (
          <li key={s.key} className="flex items-center gap-2">
            <span
              className={cn(
                'inline-flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold',
                isCurrent ? 'bg-accent text-accent-fg' : undefined,
                !isCurrent && isDone ? 'bg-success-soft text-success-soft-fg' : undefined,
                !isCurrent && !isDone ? 'bg-surface-muted text-fg-subtle' : undefined,
              )}
              aria-current={isCurrent ? 'step' : undefined}
            >
              {i + 1}
            </span>
            <span
              className={cn(
                'text-sm',
                isCurrent ? 'font-medium text-fg' : 'text-fg-subtle',
              )}
            >
              {s.label}
            </span>
            {i < STEPS.length - 1 && <span className="mx-2 text-fg-subtle">›</span>}
          </li>
        );
      })}
    </ol>
  );
}

function IdentityStep({
  composerId,
  setComposerId,
  preset,
  setPreset,
  customW,
  setCustomW,
  customH,
  setCustomH,
  fps,
  setFps,
  errors,
}: Readonly<{
  composerId: string;
  setComposerId: (v: string) => void;
  preset: CanvasPresetKey;
  setPreset: (p: CanvasPresetKey) => void;
  customW: number;
  setCustomW: (n: number) => void;
  customH: number;
  setCustomH: (n: number) => void;
  fps: number;
  setFps: (n: number) => void;
  errors: Record<string, string>;
}>) {
  return (
    <Fieldset>
      <div className="space-y-4">
        <InputField
          label="Composer ID"
          required
          placeholder="e.g. main-scene"
          value={composerId}
          onChange={(e) => setComposerId(e.target.value)}
          {...(errors.composerId ? { error: errors.composerId } : {})}
          hint="Lowercase letters, digits, and dashes. Used in upstream refs as composer:<id>."
          fullWidth
        />

        <Select
          label="Canvas preset"
          value={preset}
          onChange={(e) => setPreset(e.target.value as CanvasPresetKey)}
          required
        >
          <option value="1080p">{`1080p — ${CANVAS_PRESETS['1080p'].label}`}</option>
          <option value="1440p">{`1440p — ${CANVAS_PRESETS['1440p'].label}`}</option>
          <option value="4k">{`4K — ${CANVAS_PRESETS['4k'].label}`}</option>
          <option value="custom">Custom…</option>
        </Select>

        {preset === 'custom' && (
          <div className="grid grid-cols-2 gap-3">
            <InputField
              label="Width (px)"
              type="number"
              min={16}
              max={7680}
              step={2}
              value={customW}
              onChange={(e) => setCustomW(parseInt(e.target.value, 10) || 0)}
              {...(errors.customW ? { error: errors.customW } : {})}
              fullWidth
            />
            <InputField
              label="Height (px)"
              type="number"
              min={16}
              max={4320}
              step={2}
              value={customH}
              onChange={(e) => setCustomH(parseInt(e.target.value, 10) || 0)}
              {...(errors.customH ? { error: errors.customH } : {})}
              fullWidth
            />
          </div>
        )}

        <InputField
          label="Frame rate (fps)"
          type="number"
          min={1}
          max={240}
          step={1}
          value={fps}
          onChange={(e) => setFps(parseInt(e.target.value, 10) || 0)}
          hint="Composer render rate. Default 60. Downstream encoders inherit this."
          {...(errors.fps ? { error: errors.fps } : {})}
          fullWidth
        />
      </div>
    </Fieldset>
  );
}

function InputsStep({
  sources,
  sourcesLoading,
  selectedSourceIds,
  toggleSource,
  error,
}: Readonly<{
  sources: ComposerWizardSource[];
  sourcesLoading: boolean;
  selectedSourceIds: string[];
  toggleSource: (id: string) => void;
  error?: string;
}>) {
  const [query, setQuery] = useState('');
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return sources;
    return sources.filter((s) => {
      if (s.id.toLowerCase().includes(q)) return true;
      if (s.device && s.device.toLowerCase().includes(q)) return true;
      return false;
    });
  }, [sources, query]);

  return (
    <Fieldset>
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-medium text-fg">Pick sources to compose</h2>
          <span className="text-xs text-fg-subtle">
            {selectedSourceIds.length} selected
          </span>
        </div>

        <InputField
          placeholder="Search sources by id or device…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-label="Search sources"
          fullWidth
        />

        <SourcesBody
          sourcesLoading={sourcesLoading}
          filtered={filtered}
          totalCount={sources.length}
          selectedSourceIds={selectedSourceIds}
          toggleSource={toggleSource}
        />

        {error && (
          <p className="text-sm text-danger-soft-fg" role="alert">
            {error}
          </p>
        )}
      </div>
    </Fieldset>
  );
}

function SourcesBody({
  sourcesLoading,
  filtered,
  totalCount,
  selectedSourceIds,
  toggleSource,
}: Readonly<{
  sourcesLoading: boolean;
  filtered: ComposerWizardSource[];
  totalCount: number;
  selectedSourceIds: string[];
  toggleSource: (id: string) => void;
}>) {
  if (sourcesLoading) {
    return (
      <p className="text-sm text-fg-subtle">Loading sources…</p>
    );
  }
  if (totalCount === 0) {
    return (
      <p className="text-sm text-fg-subtle">
        No sources available. Create at least one source before building a composer.
      </p>
    );
  }
  if (filtered.length === 0) {
    return (
      <p className="text-sm text-fg-subtle">No sources match the current filter.</p>
    );
  }
  return (
    <ul className="rounded-sm border border-border divide-y divide-border max-h-72 overflow-y-auto">
      {filtered.map((s) => {
        const checked = selectedSourceIds.includes(s.id);
        const subtitle = s.test_mode
          ? 'test pattern'
          : s.device || 'unconfigured';
        return (
          <li key={s.id} className="px-3 py-2">
            <Checkbox
              checked={checked}
              onChange={() => toggleSource(s.id)}
              label={
                <span className="flex flex-col">
                  <span className="font-mono text-sm text-fg">{s.id}</span>
                  <span className="text-xs text-fg-subtle">{subtitle}</span>
                </span>
              }
            />
          </li>
        );
      })}
    </ul>
  );
}

function LayoutStep({
  canvas,
  layout,
}: Readonly<{
  canvas: ComposerCanvasDims;
  layout: ComposerLayoutSlot[];
}>) {
  return (
    <Fieldset>
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-fg">Suggested initial layout</h2>
          <span className="text-xs text-fg-subtle">
            {canvas.w}×{canvas.h} · {layout.length} input
            {layout.length === 1 ? '' : 's'}
          </span>
        </div>

        <LayoutPreview canvas={canvas} layout={layout} />

        <p className="text-xs text-fg-subtle">
          Layout is auto-suggested based on input count. Fine-tune slot
          positions and sizes in the composer layout editor after creation.
        </p>
      </div>
    </Fieldset>
  );
}

function LayoutPreview({
  canvas,
  layout,
}: Readonly<{
  canvas: ComposerCanvasDims;
  layout: ComposerLayoutSlot[];
}>) {
  const aspectRatio = canvas.w / Math.max(canvas.h, 1);
  return (
    <div
      className="relative w-full border-2 border-border rounded-md bg-surface-sunken"
      style={{ aspectRatio: `${aspectRatio}` }}
    >
      {layout.length === 0 ? (
        <div className="absolute inset-0 flex items-center justify-center text-fg-subtle text-sm">
          No inputs selected
        </div>
      ) : (
        <svg
          viewBox={`0 0 ${canvas.w} ${canvas.h}`}
          preserveAspectRatio="xMidYMid meet"
          className="absolute inset-0 w-full h-full"
          role="img"
          aria-label="Layout preview"
        >
          {layout.map((slot, i) => {
            const stroke = Math.max(3, canvas.w / 600);
            const labelSize = Math.max(12, Math.min(slot.w, slot.h) / 8);
            return (
              <g key={`${slot.input}-${i}`}>
                <rect
                  x={slot.x}
                  y={slot.y}
                  width={slot.w}
                  height={slot.h}
                  fill="rgba(59, 130, 246, 0.18)"
                  stroke="#3b82f6"
                  strokeWidth={stroke}
                />
                <text
                  x={slot.x + slot.w / 2}
                  y={slot.y + slot.h / 2}
                  textAnchor="middle"
                  dominantBaseline="middle"
                  fill="#ffffff"
                  fontSize={labelSize}
                  fontFamily="monospace"
                >
                  {slot.input}
                </text>
              </g>
            );
          })}
        </svg>
      )}
    </div>
  );
}

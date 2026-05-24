import { InputField } from '../InputField';
import { Select } from '../Select';
import { Button } from '../Button';
import type { PublishTarget, PublishType } from './types';

interface PublishTargetsEditorProps {
  value: PublishTarget[];
  onChange: (next: PublishTarget[]) => void;
  disabled?: boolean;
  errors: Record<string, string>;
}

const TYPES: { value: PublishType; label: string; placeholder: string }[] = [
  { value: 'rtsp', label: 'RTSP', placeholder: 'rtsp://host:8554/path' },
  { value: 'srt', label: 'SRT', placeholder: 'srt://host:9000?streamid=publish/key' },
  { value: 'hls', label: 'HLS', placeholder: 'https://host/path/index.m3u8' },
];

function placeholderFor(type: PublishType) {
  return TYPES.find((t) => t.value === type)?.placeholder ?? '';
}

export function PublishTargetsEditor({
  value,
  onChange,
  disabled,
  errors,
}: Readonly<PublishTargetsEditorProps>) {
  const addRow = () => {
    onChange([...value, { type: 'rtsp', url: '' }]);
  };

  const removeRow = (index: number) => {
    onChange(value.filter((_, i) => i !== index));
  };

  const updateRow = (index: number, patch: Partial<PublishTarget>) => {
    onChange(value.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  };

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-fg">Publish Targets</h2>
        <Button
          theme="light"
          size="SM"
          type="button"
          onClick={addRow}
          disabled={disabled}
          text="+ Add target"
        />
      </div>
      {value.length === 0 ? (
        <p className="text-sm text-fg-subtle">
          No external publish targets. RTSP/SRT/WebRTC endpoints from the daemon are still served.
        </p>
      ) : (
        <ul className="space-y-3">
          {value.map((target, index) => {
            const urlErr = errors[`publish.${index}.url`];
            return (
              <li
                key={index}
                className="grid grid-cols-[8rem_1fr_auto] gap-3 items-start p-3 border border-border rounded-md bg-surface-muted"
              >
                <Select
                  label="Type"
                  value={target.type}
                  onChange={(e) => updateRow(index, { type: e.target.value as PublishType })}
                  disabled={disabled}
                  fullWidth
                >
                  {TYPES.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </Select>
                <InputField
                  label="URL"
                  type="text"
                  value={target.url}
                  onChange={(e) => updateRow(index, { url: e.target.value })}
                  placeholder={placeholderFor(target.type)}
                  disabled={disabled}
                  {...(urlErr ? { error: urlErr } : {})}
                />
                <div className="pt-6">
                  <Button
                    theme="danger"
                    size="SM"
                    type="button"
                    onClick={() => removeRow(index)}
                    disabled={disabled}
                    text="Remove"
                  />
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

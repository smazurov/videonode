import type { UseDeviceInputFormResult } from '../hooks/useDeviceInputForm';
import { Spinner } from './Spinner';

interface DeviceInputCapabilitiesProps {
  form: UseDeviceInputFormResult;
  mode: 'create' | 'edit';
  disabled?: boolean;
  errors?: Record<string, string>;
}

const selectClasses =
  'block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent';

function LoadingSpinner() {
  return (
    <div className="flex items-center space-x-2 p-3 border border-border rounded-md bg-surface-muted">
      <Spinner size="sm" />
      <span className="text-sm text-fg-muted">Loading...</span>
    </div>
  );
}

export function DeviceInputCapabilities({
  form,
  mode,
  disabled = false,
  errors = {},
}: Readonly<DeviceInputCapabilitiesProps>) {
  const handleResolutionChange = (value: string) => {
    if (value === '') {
      form.selectResolution(0, 0);
      return;
    }
    const [w, h] = value.split('x').map(Number);
    if (w && h) form.selectResolution(w, h);
  };

  const handleFramerateChange = (value: string) => {
    form.setFramerate(value === '' ? 0 : Number.parseInt(value, 10));
  };

  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
      {/* Input Format */}
      <div>
        <label className="block text-sm font-medium text-fg mb-2">
          Input Format {mode === 'create' && <span className="text-danger">*</span>}
        </label>
        {form.formatsLoading && mode === 'create' ? (
          <LoadingSpinner />
        ) : (
          <select
            value={form.inputFormat}
            onChange={(e) => form.selectFormat(e.target.value)}
            className={selectClasses}
            disabled={disabled || (mode === 'create' && form.formatOptions.length === 0)}
            required={mode === 'create'}
          >
            <option value="">{mode === 'edit' ? 'Auto' : 'Select format...'}</option>
            {form.formatOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        )}
        {errors.input_format && (
          <p className="mt-1 text-sm text-danger-soft-fg">{errors.input_format}</p>
        )}
      </div>

      {/* Resolution */}
      <div>
        <label className="block text-sm font-medium text-fg mb-2">
          Resolution
        </label>
        {form.resolutionsLoading && form.inputFormat && mode === 'create' ? (
          <LoadingSpinner />
        ) : (
          <select
            value={form.width && form.height ? `${form.width}x${form.height}` : ''}
            onChange={(e) => handleResolutionChange(e.target.value)}
            className={selectClasses}
            disabled={disabled}
          >
            <option value="">Auto</option>
            {!form.inputFormat && mode === 'create' && (
              <option disabled>Select format first to see resolutions</option>
            )}
            {form.resolutionOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        )}
      </div>

      {/* Framerate */}
      <div>
        <label className="block text-sm font-medium text-fg mb-2">
          Framerate
        </label>
        {form.frameratesLoading && form.width > 0 && form.height > 0 && mode === 'create' ? (
          <LoadingSpinner />
        ) : (
          <select
            value={form.framerate ? form.framerate.toString() : ''}
            onChange={(e) => handleFramerateChange(e.target.value)}
            className={selectClasses}
            disabled={disabled}
          >
            <option value="">Auto</option>
            {form.framerateOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        )}
      </div>
    </div>
  );
}

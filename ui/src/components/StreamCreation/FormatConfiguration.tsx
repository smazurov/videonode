import { 
  useDeviceFormats, 
  useDeviceResolutions, 
  useDeviceFramerates 
} from '../../hooks/useDeviceCapabilities';
import { COMMON_FRAMERATES, RESOLUTION_LABELS, COMMON_RESOLUTION_CONFIGS } from './constants';

interface FormatConfigurationProps {
  deviceId: string;
  selectedFormat: string;
  width?: number | undefined;
  height?: number | undefined;
  framerate?: number | undefined;
  onFormatChange: (format: string) => void;
  onResolutionChange: (width?: number | undefined, height?: number | undefined) => void;
  onFramerateChange: (fps?: number | undefined) => void;
  disabled?: boolean;
  errors?: {
    framerate?: string | undefined;
    width?: string | undefined;
    height?: string | undefined;
  };
}

export function FormatConfiguration({
  deviceId,
  selectedFormat,
  width,
  height,
  framerate,
  onFormatChange,
  onResolutionChange,
  onFramerateChange,
  disabled = false,
  errors = {}
}: Readonly<FormatConfigurationProps>) {
  const { formats, loading: loadingFormats } = useDeviceFormats(deviceId);
  const { resolutions, loading: loadingResolutions, error: resolutionsError } = useDeviceResolutions(deviceId, selectedFormat);
  const { framerates, loading: loadingFramerates, error: frameratesError } = useDeviceFramerates(
    deviceId,
    selectedFormat,
    width,
    height
  );
  
  const currentResolution = width && height ? `${width}x${height}` : '';
  
  const handleResolutionChange = (resolution: string) => {
    if (resolution === '') {
      onResolutionChange(undefined, undefined);
    } else {
      const [w, h] = resolution.split('x').map(Number);
      onResolutionChange(w, h);
    }
  };
  
  const handleFramerateChange = (value: string) => {
    onFramerateChange(value ? parseInt(value, 10) : undefined);
  };
  
  if (formats.length === 0) {
    return null;
  }
  
  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
      {/* Input Format Selection */}
      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Input Format <span className="text-red-500">*</span>
        </label>
        {loadingFormats ? (
          <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
            <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
          </div>
        ) : (
          <select
            value={selectedFormat}
            onChange={(e) => {
              onFormatChange(e.target.value);
              // Auto-selection will handle resolution and framerate
            }}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
            disabled={disabled || formats.length === 0}
            required
          >
            <option value="">Select format...</option>
            {formats.map((format) => (
              <option key={format.format_name} value={format.format_name}>
                {format.format_name.toUpperCase()} - {format.original_name}
              </option>
            ))}
          </select>
        )}
      </div>

      {/* Resolution Selection */}
      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Resolution
        </label>
        {!selectedFormat ? (
          <select
            disabled
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm bg-gray-100 dark:bg-gray-900 cursor-not-allowed"
          >
            <option>Select format first</option>
          </select>
        ) : loadingResolutions ? (
          <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
            <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
          </div>
        ) : resolutionsError ? (
          <div className="p-3 border border-red-300 dark:border-red-600 rounded-md bg-red-50 dark:bg-red-900/20">
            <p className="text-sm text-red-600 dark:text-red-400">Error</p>
          </div>
        ) : (
          <select
            value={currentResolution}
            onChange={(e) => handleResolutionChange(e.target.value)}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
            disabled={disabled || resolutions.length === 0}
          >
            <option value="">Auto</option>
            {resolutions
              .filter((res) => {
                // Only show common resolutions
                return COMMON_RESOLUTION_CONFIGS.some(cr => cr.width === res.width && cr.height === res.height);
              })
              .map((res) => {
                const resString = `${res.width}x${res.height}`;
                const label = RESOLUTION_LABELS[resString] 
                  ? `${resString} (${RESOLUTION_LABELS[resString]})` 
                  : resString;
                
                return (
                  <option key={resString} value={resString}>
                    {label}
                  </option>
                );
              })}
          </select>
        )}
        {errors.width && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.width}</p>
        )}
        {errors.height && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.height}</p>
        )}
      </div>

      {/* Framerate Selection */}
      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Framerate (FPS)
        </label>
        {!selectedFormat || !width || !height ? (
          <select
            value={framerate?.toString() || ''}
            onChange={(e) => handleFramerateChange(e.target.value)}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
            disabled={disabled}
          >
            {COMMON_FRAMERATES.map((fr) => (
              <option key={fr.value} value={fr.value}>
                {fr.label}
              </option>
            ))}
          </select>
        ) : loadingFramerates ? (
          <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
            <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
          </div>
        ) : frameratesError ? (
          <div className="p-3 border border-red-300 dark:border-red-600 rounded-md bg-red-50 dark:bg-red-900/20">
            <p className="text-sm text-red-600 dark:text-red-400">Error</p>
          </div>
        ) : (
          <select
            value={framerate?.toString() || ''}
            onChange={(e) => handleFramerateChange(e.target.value)}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
            disabled={disabled || framerates.length === 0}
          >
            <option value="">Auto</option>
            {framerates.map((rate) => {
              const fpsValue = Math.round(rate.fps);
              return (
                <option key={`${rate.numerator}/${rate.denominator}`} value={fpsValue.toString()}>
                  {fpsValue} FPS ({rate.numerator}/{rate.denominator})
                </option>
              );
            })}
          </select>
        )}
        {errors.framerate && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.framerate}</p>
        )}
      </div>
    </div>
  );
}
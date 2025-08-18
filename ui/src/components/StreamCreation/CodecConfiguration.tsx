import { COMMON_CODECS, COMMON_BITRATES } from './constants';

interface CodecConfigurationProps {
  codec: string;
  bitrate?: number | undefined;
  onCodecChange: (codec: string) => void;
  onBitrateChange: (bitrate?: number | undefined) => void;
  disabled?: boolean;
  errors?: {
    codec?: string | undefined;
    bitrate?: string | undefined;
  };
}

export function CodecConfiguration({
  codec,
  bitrate,
  onCodecChange,
  onBitrateChange,
  disabled = false,
  errors = {}
}: Readonly<CodecConfigurationProps>) {
  
  const handleBitrateChange = (value: string) => {
    onBitrateChange(value ? parseInt(value, 10) : undefined);
  };
  
  return (
    <>
      {/* Codec Selection */}
      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Video Codec <span className="text-red-500">*</span>
        </label>
        <select
          value={codec}
          onChange={(e) => onCodecChange(e.target.value)}
          className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
          disabled={disabled}
          required
        >
          {COMMON_CODECS.map((codecOption) => (
            <option key={codecOption.value} value={codecOption.value}>
              {codecOption.label}
            </option>
          ))}
        </select>
        {errors.codec && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.codec}</p>
        )}
      </div>
      
      {/* Bitrate Selection */}
      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Bitrate (kbps)
        </label>
        <select
          value={bitrate?.toString() || ''}
          onChange={(e) => handleBitrateChange(e.target.value)}
          className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
          disabled={disabled}
        >
          {COMMON_BITRATES.map((bitrateOption) => (
            <option key={bitrateOption.value} value={bitrateOption.value}>
              {bitrateOption.label}
            </option>
          ))}
        </select>
        {errors.bitrate && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.bitrate}</p>
        )}
      </div>
    </>
  );
}
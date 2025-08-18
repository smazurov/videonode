import { useEffect } from 'react';
import { StreamRequestData } from '../lib/api';
import { 
  useDeviceFormats, 
  useDeviceResolutions, 
  useDeviceFramerates 
} from './useDeviceCapabilities';

interface UseDeviceAutoSelectProps {
  deviceId: string;
  selectedFormat: string;
  formData: StreamRequestData;
  setSelectedFormat: (format: string) => void;
  setFormData: React.Dispatch<React.SetStateAction<StreamRequestData>>;
}

export function useDeviceAutoSelect({
  deviceId,
  selectedFormat,
  formData,
  setSelectedFormat,
  setFormData
}: UseDeviceAutoSelectProps) {
  const { formats } = useDeviceFormats(deviceId);
  const { resolutions } = useDeviceResolutions(deviceId, selectedFormat);
  const { framerates } = useDeviceFramerates(
    deviceId,
    selectedFormat,
    formData.width,
    formData.height
  );

  // Auto-select first non-emulated format when formats are loaded
  useEffect(() => {
    if (formats.length > 0 && !selectedFormat) {
      // Prefer non-emulated formats
      const nativeFormat = formats.find(f => !f.emulated);
      const format = nativeFormat || formats[0];
      if (format) {
        console.log(`Auto-selecting format: ${format.format_name} (${format.original_name})`);
        setSelectedFormat(format.format_name);
        setFormData(prev => ({
          ...prev,
          input_format: format.format_name
        }));
      }
    }
  }, [formats, selectedFormat, setSelectedFormat, setFormData]);

  // Auto-select highest resolution when format changes and resolutions are loaded
  useEffect(() => {
    // When format changes and we have resolutions, select the highest
    if (selectedFormat && resolutions.length > 0) {
      // Log all available resolutions
      console.log(`Available resolutions for format '${selectedFormat}':`, 
        resolutions.map(r => `${r.width}x${r.height}`).join(', '));
      
      // Sort by total pixels and pick the highest
      const highest = [...resolutions].sort((a, b) => 
        (b.width * b.height) - (a.width * a.height)
      )[0];
      
      if (highest) {
        console.log(`Auto-selecting highest resolution: ${highest.width}x${highest.height}`);
        
        setFormData(prev => ({
          ...prev,
          width: highest.width,
          height: highest.height
        }));
      }
    }
  }, [selectedFormat, resolutions]);

  // Auto-select highest framerate when resolution is set and framerates are loaded
  useEffect(() => {
    // When we have resolution and framerates, select the highest FPS
    if (formData.width && formData.height && framerates.length > 0) {
      // Log all available framerates
      console.log(`Available framerates for ${formData.width}x${formData.height}:`, 
        framerates.map(f => `${Math.round(f.fps)} FPS`).join(', '));
      
      const highest = [...framerates].sort((a, b) => b.fps - a.fps)[0];
      
      if (highest) {
        console.log(`Auto-selecting highest FPS: ${Math.round(highest.fps)}`);
        
        setFormData(prev => ({
          ...prev,
          framerate: Math.round(highest.fps)
        }));
      }
    }
  }, [formData.width, formData.height, framerates]);

  // Clear format selection when device changes
  useEffect(() => {
    setSelectedFormat('');
    // Also clear resolution and framerate when device changes
    setFormData(prev => ({
      ...prev,
      width: undefined,
      height: undefined,
      framerate: undefined,
      input_format: ''
    }));
  }, [deviceId, setSelectedFormat, setFormData]);

  return {
    formats,
    resolutions,
    framerates
  };
}
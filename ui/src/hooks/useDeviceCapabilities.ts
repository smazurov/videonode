import { useState, useEffect } from 'react';
import {
  FormatInfo,
  Resolution,
  Framerate,
  DeviceCapabilitiesData,
  DeviceResolutionsData,
  DeviceFrameratesData,
  getDeviceFormats,
  getDeviceResolutions,
  getDeviceFramerates,
} from '../lib/api';

// Request deduplication and cancellation management
const requestCache = new Map<string, Promise<unknown>>();
const activeControllers = new Map<string, AbortController>();

export interface DeviceFrameratesResult {
  framerates: Framerate[];
  loading: boolean;
  error: string | null;
}

export function useDeviceFormats(deviceId: string) {
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!deviceId) {
      setFormats([]);
      return;
    }

    const cacheKey = `formats:${deviceId}`;
    
    // Cancel previous request
    activeControllers.get(cacheKey)?.abort();
    
    const controller = new AbortController();
    activeControllers.set(cacheKey, controller);

    const fetchFormats = async () => {
      setLoading(true);
      setError(null);
      
      try {
        // Check if request already in flight
        if (!requestCache.has(cacheKey)) {
          requestCache.set(cacheKey, getDeviceFormats(deviceId));
        }
        
        const data = await requestCache.get(cacheKey) as DeviceCapabilitiesData;
        
        if (!controller.signal.aborted) {
          setFormats(data.formats);
        }
      } catch (error_: unknown) {
        const errorObj = error_ as { name?: string };
        if (!controller.signal.aborted && errorObj?.name !== 'AbortError') {
          setError(error_ instanceof Error ? error_.message : 'Failed to fetch formats');
          setFormats([]);
        }
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          requestCache.delete(cacheKey);
        }
      }
    };

    fetchFormats();

    return () => {
      controller.abort();
      activeControllers.delete(cacheKey);
    };
  }, [deviceId]);

  return { formats, loading, error };
}

export function useDeviceResolutions(deviceId: string, formatName: string) {
  const [resolutions, setResolutions] = useState<Resolution[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!deviceId || !formatName) {
      setResolutions([]);
      setLoading(false);
      return;
    }

    // Clear immediately on parameter change
    setResolutions([]);
    
    const cacheKey = `resolutions:${deviceId}:${formatName}`;
    
    // Cancel previous request
    activeControllers.get(cacheKey)?.abort();
    
    const controller = new AbortController();
    activeControllers.set(cacheKey, controller);

    const fetchResolutions = async () => {
      setLoading(true);
      setError(null);
      
      try {
        // Check if request already in flight
        if (!requestCache.has(cacheKey)) {
          requestCache.set(cacheKey, getDeviceResolutions(deviceId, formatName));
        }
        
        const data = await requestCache.get(cacheKey) as DeviceResolutionsData;
        
        if (!controller.signal.aborted) {
          setResolutions(data.resolutions);
        }
      } catch (error_: unknown) {
        const errorObj = error_ as { name?: string };
        if (!controller.signal.aborted && errorObj?.name !== 'AbortError') {
          setError(error_ instanceof Error ? error_.message : 'Failed to fetch resolutions');
          setResolutions([]);
        }
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          requestCache.delete(cacheKey);
        }
      }
    };

    fetchResolutions();

    return () => {
      controller.abort();
      activeControllers.delete(cacheKey);
    };
  }, [deviceId, formatName]);

  return { resolutions, loading, error };
}

export function useDeviceFramerates(
  deviceId: string | undefined,
  formatName: string | undefined,
  width: number | undefined,
  height: number | undefined
): DeviceFrameratesResult {
  const [framerates, setFramerates] = useState<Framerate[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // More strict validation
    if (!deviceId || !formatName || !width || !height || width === 0 || height === 0) {
      setFramerates([]);
      setLoading(false);
      return;
    }

    const cacheKey = `framerates:${deviceId}:${formatName}:${width}x${height}`;
    
    // Cancel previous request
    activeControllers.get(cacheKey)?.abort();
    
    const controller = new AbortController();
    activeControllers.set(cacheKey, controller);

    const fetchFramerates = async () => {
      setLoading(true);
      setError(null);
      
      try {
        // Check if request already in flight
        if (!requestCache.has(cacheKey)) {
          requestCache.set(
            cacheKey, 
            getDeviceFramerates(deviceId, formatName, width, height)
          );
        }
        
        const data = await requestCache.get(cacheKey) as DeviceFrameratesData;
        
        if (!controller.signal.aborted) {
          setFramerates(data.framerates);
        }
      } catch (error_: unknown) {
        const errorObj = error_ as { name?: string };
        if (!controller.signal.aborted && errorObj?.name !== 'AbortError') {
          // Don't set error for expected 400/500 errors (invalid resolution)
          const errorMessage = error_ instanceof Error ? error_.message : String(error_);
          if (errorMessage.includes('400') || errorMessage.includes('500')) {
            setFramerates([]);
            setError(null); // Silent fail for invalid combinations
          } else {
            setError(errorMessage);
            setFramerates([]);
          }
        }
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          requestCache.delete(cacheKey);
        }
      }
    };

    fetchFramerates();

    return () => {
      controller.abort();
      activeControllers.delete(cacheKey);
    };
  }, [deviceId, formatName, width, height]);

  return { framerates, loading, error };
}
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
  getDeviceFramerates
} from '../lib/api';

export function useDeviceFormats(deviceId: string) {
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!deviceId) {
      setFormats([]);
      return;
    }

    let cancelled = false;

    const fetchFormats = async () => {
      setLoading(true);
      setError(null);
      
      try {
        const data = await getDeviceFormats(deviceId);
        if (!cancelled) {
          setFormats(data.formats);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to fetch formats');
          setFormats([]);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchFormats();

    return () => {
      cancelled = true;
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
      return;
    }

    let cancelled = false;

    const fetchResolutions = async () => {
      setLoading(true);
      setError(null);
      
      try {
        const data = await getDeviceResolutions(deviceId, formatName);
        if (!cancelled) {
          setResolutions(data.resolutions);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to fetch resolutions');
          setResolutions([]);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchResolutions();

    return () => {
      cancelled = true;
    };
  }, [deviceId, formatName]);

  return { resolutions, loading, error };
}

export function useDeviceFramerates(
  deviceId: string, 
  formatName: string, 
  width: number | undefined, 
  height: number | undefined
) {
  const [framerates, setFramerates] = useState<Framerate[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!deviceId || !formatName || !width || !height) {
      setFramerates([]);
      return;
    }

    let cancelled = false;

    const fetchFramerates = async () => {
      setLoading(true);
      setError(null);
      
      try {
        const data = await getDeviceFramerates(deviceId, formatName, width, height);
        if (!cancelled) {
          setFramerates(data.framerates);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to fetch framerates');
          setFramerates([]);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchFramerates();

    return () => {
      cancelled = true;
    };
  }, [deviceId, formatName, width, height]);

  return { framerates, loading, error };
}
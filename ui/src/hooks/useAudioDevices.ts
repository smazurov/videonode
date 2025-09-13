import { useState, useEffect } from 'react';
import { getAudioDevices, AudioDevice } from '../lib/api';

export function useAudioDevices() {
  const [devices, setDevices] = useState<AudioDevice[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;

    const fetchDevices = async () => {
      setLoading(true);
      setError(null);
      
      try {
        const data = await getAudioDevices();
        if (mounted) {
          setDevices(data.devices);
        }
      } catch (error_) {
        if (mounted) {
          setError(error_ instanceof Error ? error_.message : 'Failed to load audio devices');
          console.error('Failed to fetch audio devices:', error_);
        }
      } finally {
        if (mounted) {
          setLoading(false);
        }
      }
    };

    fetchDevices();

    return () => {
      mounted = false;
    };
  }, []);

  return { devices, loading, error };
}
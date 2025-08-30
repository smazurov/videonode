import { useState, useEffect } from 'react';
import { API_BASE_URL, type VersionInfo } from '../lib/api';

export function useVersion() {
  const [version, setVersion] = useState<VersionInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Direct fetch without auth since /api/version doesn't require it
    fetch(`${API_BASE_URL}/api/version`)
      .then(res => {
        if (!res.ok) throw new Error(`Failed to fetch version: ${res.statusText}`);
        return res.json();
      })
      .then(data => {
        setVersion(data);
        setError(null);
      })
      .catch(err => {
        console.error('Version fetch error:', err);
        setError(err.message);
        setVersion(null);
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  return { version, loading, error };
}
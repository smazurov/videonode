import { useState, useEffect } from 'react';
import { API_BASE_URL } from '../lib/api';
import type { components } from '../lib/api.generated';

type VersionData = components["schemas"]["VersionData"];

export function useVersion() {
  const [version, setVersion] = useState<VersionData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Direct fetch without auth since /api/update/version doesn't require it
    fetch(`${API_BASE_URL}/api/update/version`)
      .then(res => {
        if (!res.ok) throw new Error(`Failed to fetch version: ${res.statusText}`);
        return res.json();
      })
      .then((data: VersionData) => {
        setVersion(data);
        setError(null);
      })
      .catch((error_: Error) => {
        console.error('Version fetch error:', error_);
        setError(error_.message);
        setVersion(null);
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  return { version, loading, error };
}
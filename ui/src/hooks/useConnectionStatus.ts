import { useState } from 'react';
import { useSSEManager, getGlobalConnectionStatus, type ConnectionStatus } from './useSSEManager';

// useConnectionStatus exposes the shared SSE client's connection status as
// local state. It seeds from the current global status so a consumer mounting
// during a healthy session doesn't flash a disconnected indicator, then tracks
// live changes via the SSE manager's connection callback.
export function useConnectionStatus(): ConnectionStatus {
  const [status, setStatus] = useState<ConnectionStatus>(getGlobalConnectionStatus);
  useSSEManager({ onConnectionStatusChange: setStatus });
  return status;
}

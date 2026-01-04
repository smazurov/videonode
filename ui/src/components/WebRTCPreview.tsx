import { useEffect, useRef, useState, useCallback } from 'react';
import { webrtcSignaling } from '../lib/api';

// Wait for ICE gathering to complete with timeout
function waitForIceGathering(pc: RTCPeerConnection, timeoutMs: number): Promise<void> {
  return new Promise((resolve) => {
    if (pc.iceGatheringState === 'complete') {
      resolve();
      return;
    }
    const timeout = setTimeout(() => resolve(), timeoutMs);
    pc.onicegatheringstatechange = () => {
      if (pc.iceGatheringState === 'complete') {
        clearTimeout(timeout);
        resolve();
      }
    };
  });
}

interface WebRTCPreviewProps {
  streamId: string;
  className?: string;
  refreshKey?: number;
}

type ConnectionState = 'idle' | 'connecting' | 'connected' | 'failed' | 'closed';

export function WebRTCPreview({ streamId, className = '', refreshKey = 0 }: Readonly<WebRTCPreviewProps>) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const [connectionState, setConnectionState] = useState<ConnectionState>('idle');
  const [errorMessage, setErrorMessage] = useState<string>('');
  const [retryCounter, setRetryCounter] = useState(0);

  const cleanup = useCallback(() => {
    if (pcRef.current) {
      pcRef.current.close();
      pcRef.current = null;
    }
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }
    streamRef.current = null;
  }, []);

  // Connect on mount and when refreshKey/retryCounter changes
  useEffect(() => {
    let cancelled = false;

    const startConnection = async () => {
      cleanup();
      if (cancelled) return;
      setConnectionState('connecting');
      setErrorMessage('');

      try {
        // Create peer connection
        const pc = new RTCPeerConnection({
          iceServers: [], // LAN-only, no STUN/TURN needed
        });
        if (cancelled) {
          pc.close();
          return;
        }
        pcRef.current = pc;

        // Handle incoming tracks - store stream and attach if video exists
        pc.ontrack = (event) => {
          if (event.streams[0]) {
            streamRef.current = event.streams[0];
            if (videoRef.current) {
              videoRef.current.srcObject = event.streams[0];
            }
          }
        };

        // Monitor connection state
        pc.onconnectionstatechange = () => {
          if (cancelled) return;
          switch (pc.connectionState) {
            case 'connected':
              setConnectionState('connected');
              break;
            case 'disconnected':
            case 'failed':
              setConnectionState('failed');
              setErrorMessage('Connection lost');
              break;
            case 'closed':
              setConnectionState('closed');
              break;
          }
        };

        // Add transceivers for receiving video and audio
        pc.addTransceiver('video', { direction: 'recvonly' });
        pc.addTransceiver('audio', { direction: 'recvonly' });

        // Create offer
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);

        // Wait for ICE gathering to complete (or timeout after 1s for LAN)
        await waitForIceGathering(pc, 1000);

        if (cancelled) {
          pc.close();
          return;
        }

        // Send offer to server
        const answerSdp = await webrtcSignaling(streamId, pc.localDescription?.sdp || '');

        if (cancelled) {
          pc.close();
          return;
        }

        // Set remote description
        await pc.setRemoteDescription({
          type: 'answer',
          sdp: answerSdp,
        });

      } catch (error) {
        if (cancelled) return;
        console.error('WebRTC connection failed:', error);
        setConnectionState('failed');
        setErrorMessage(error instanceof Error ? error.message : 'Connection failed');
        cleanup();
      }
    };

    startConnection();

    return () => {
      cancelled = true;
      cleanup();
    };
  }, [streamId, refreshKey, retryCounter, cleanup]);

  // Attach stream when video element mounts (fixes race condition)
  useEffect(() => {
    if (connectionState === 'connected' && videoRef.current && streamRef.current) {
      videoRef.current.srcObject = streamRef.current;
    }
  }, [connectionState]);

  const handleRetry = () => {
    setRetryCounter(c => c + 1);
  };

  if (connectionState === 'idle' || connectionState === 'connecting') {
    return (
      <div className={`flex items-center justify-center bg-gray-100 dark:bg-gray-800 ${className}`}>
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-2"></div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Connecting...</p>
        </div>
      </div>
    );
  }

  if (connectionState === 'failed' || connectionState === 'closed') {
    return (
      <div className={`flex items-center justify-center bg-gray-100 dark:bg-gray-800 ${className}`}>
        <div className="text-center">
          <div className="w-16 h-16 bg-red-100 dark:bg-red-900 rounded-lg flex items-center justify-center mb-2 mx-auto">
            <svg
              className="w-8 h-8 text-red-500 dark:text-red-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
          </div>
          <p className="text-sm text-red-600 dark:text-red-400">
            {errorMessage || 'Stream unavailable'}
          </p>
          <button
            onClick={handleRetry}
            className="text-xs text-blue-600 dark:text-blue-400 hover:underline mt-1"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={`relative bg-black ${className}`}>
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        className="w-full h-full object-contain"
      />
    </div>
  );
}

import { useEffect, useRef, useState } from 'react';
import { webrtcSignaling } from '../../lib/api';
import { StatsOverlay } from './StatsOverlay';

const RECONNECT_DELAY_MS = 2000;
const ICE_GATHER_TIMEOUT_MS = 2000;

interface Props {
  readonly streamId: string;
  readonly className?: string;
  readonly muted?: boolean;
  readonly showStats?: boolean;
}

function waitForIceGathering(pc: RTCPeerConnection, timeoutMs: number): Promise<void> {
  return new Promise((resolve) => {
    if (pc.iceGatheringState === 'complete') {
      resolve();
      return;
    }
    const onStateChange = () => {
      if (pc.iceGatheringState === 'complete') {
        pc.removeEventListener('icegatheringstatechange', onStateChange);
        resolve();
      }
    };
    pc.addEventListener('icegatheringstatechange', onStateChange);
    setTimeout(resolve, timeoutMs);
  });
}

type ConnectionState = 'connecting' | 'connected' | 'offline';

export function WebRTCPlayer({ streamId, className = '', muted = true, showStats = false }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const [pc, setPC] = useState<RTCPeerConnection | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting');
  const [playBlocked, setPlayBlocked] = useState(false);
  const reconnectTimer = useRef<number | null>(null);

  useEffect(() => {
    if (typeof RTCPeerConnection === 'undefined') {
      queueMicrotask(() => setError('WebRTC not supported in this browser'));
      return;
    }

    let cancelled = false;

    const onPlayBlocked = () => setPlayBlocked(true);

    function scheduleReconnect() {
      if (reconnectTimer.current) return;
      reconnectTimer.current = window.setTimeout(() => {
        reconnectTimer.current = null;
        if (!cancelled) connect();
      }, RECONNECT_DELAY_MS);
    }

    async function connect() {
      setConnectionState('connecting');

      if (pcRef.current) {
        pcRef.current.close();
        pcRef.current = null;
      }

      const peerConnection = new RTCPeerConnection({ iceServers: [] });
      pcRef.current = peerConnection;
      setPC(peerConnection);

      peerConnection.ontrack = (e) => {
        if (!videoRef.current || !e.streams[0]) return;
        videoRef.current.srcObject = e.streams[0];
        videoRef.current.play().catch(onPlayBlocked);
      };

      peerConnection.onconnectionstatechange = () => {
        if (cancelled) return;
        const state = peerConnection.connectionState;
        if (state === 'connected') {
          setConnectionState('connected');
        } else if (state === 'failed' || state === 'disconnected') {
          setConnectionState('offline');
          scheduleReconnect();
        }
      };

      peerConnection.addTransceiver('video', { direction: 'recvonly' });
      peerConnection.addTransceiver('audio', { direction: 'recvonly' });

      try {
        const offer = await peerConnection.createOffer();
        await peerConnection.setLocalDescription(offer);
        await waitForIceGathering(peerConnection, ICE_GATHER_TIMEOUT_MS);

        if (cancelled) return;

        const answer = await webrtcSignaling(streamId, peerConnection.localDescription!.sdp);
        if (cancelled) return;

        await peerConnection.setRemoteDescription({ type: 'answer', sdp: answer });
      } catch (error_) {
        console.error(`WebRTC [${streamId}]: connection failed`, error_);
        if (!cancelled) {
          setConnectionState('offline');
          scheduleReconnect();
        }
      }
    }

    connect();

    const videoElement = videoRef.current;

    return () => {
      cancelled = true;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      if (pcRef.current) {
        pcRef.current.close();
        pcRef.current = null;
        setPC(null);
      }
      if (videoElement) {
        videoElement.srcObject = null;
      }
    };
  }, [streamId]);

  if (error) {
    return (
      <div className={`relative flex items-center justify-center ${className}`} style={{ background: '#000' }}>
        <span className="text-red-400 text-sm">{error}</span>
      </div>
    );
  }

  const isOffline = connectionState === 'offline';
  const isConnecting = connectionState === 'connecting';

  const handleClickToPlay = () => {
    if (videoRef.current) {
      videoRef.current.play()
        .then(() => setPlayBlocked(false))
        .catch(console.error);
    }
  };

  return (
    <div className={`relative ${className}`} style={{ background: '#000' }}>
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted={muted}
        className="w-full h-full object-contain"
      />
      {isOffline && (
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-gray-400 text-sm">Stream offline</span>
        </div>
      )}
      {isConnecting && (
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-gray-400 text-sm">Connecting...</span>
        </div>
      )}
      {showStats && connectionState === 'connected' && (
        <StatsOverlay pc={pc} videoRef={videoRef} streamId={streamId} />
      )}
      {playBlocked && (
        <div
          className="absolute inset-0 flex items-center justify-center cursor-pointer bg-black/50"
          onClick={handleClickToPlay}
        >
          <div className="text-white text-center">
            <svg className="w-16 h-16 mx-auto" viewBox="0 0 24 24" fill="currentColor">
              <path d="M8 5v14l11-7z" />
            </svg>
            <span className="text-sm mt-2 block">Click to play</span>
          </div>
        </div>
      )}
    </div>
  );
}

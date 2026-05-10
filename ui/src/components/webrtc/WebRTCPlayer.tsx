import { useEffect, useRef, useState } from 'react';
import { PlayIcon } from '@heroicons/react/24/outline';
import { webrtcSignaling } from '../../lib/api';
import { StatsOverlay } from './StatsOverlay';

const RECONNECT_BASE_MS = 2000;
const RECONNECT_MAX_MS = 30_000;
const ICE_GATHER_TIMEOUT_MS = 2000;

interface Props {
  readonly streamId: string;
  readonly className?: string;
  readonly muted?: boolean;
  readonly showStats?: boolean;
}

type ConnectionState = 'connecting' | 'connected' | 'offline';

function extractPeerId(sdp: string): string | null {
  const result = /a=ice-ufrag:(\S+)/.exec(sdp);
  return result?.[1] ?? null;
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

interface PeerConnectionCallbacks {
  onConnected: () => void;
  onOffline: () => void;
  onStream: (stream: MediaStream) => void;
}

function attachPeerHandlers(
  pc: RTCPeerConnection,
  cancelledRef: React.RefObject<boolean>,
  callbacks: PeerConnectionCallbacks,
  scheduleReconnect: () => void
): void {
  const stream = new MediaStream();
  let streamDelivered = false;

  pc.ontrack = (e) => {
    stream.addTrack(e.track);
    if (!streamDelivered) {
      streamDelivered = true;
      callbacks.onStream(stream);
    }
  };

  pc.onconnectionstatechange = () => {
    if (cancelledRef.current) return;
    const state = pc.connectionState;
    if (state === 'connected') {
      callbacks.onConnected();
    } else if (state === 'failed' || state === 'disconnected') {
      callbacks.onOffline();
      scheduleReconnect();
    }
  };
}

async function performSignaling(
  pc: RTCPeerConnection,
  streamId: string,
  cancelledRef: React.RefObject<boolean>
): Promise<string | null> {
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await waitForIceGathering(pc, ICE_GATHER_TIMEOUT_MS);

  if (cancelledRef.current) return null;

  const answer = await webrtcSignaling(streamId, pc.localDescription!.sdp);
  if (cancelledRef.current) return null;

  await pc.setRemoteDescription({ type: 'answer', sdp: answer });
  return extractPeerId(answer);
}

export function WebRTCPlayer({ streamId, className = '', muted = true, showStats = false }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const cancelledRef = useRef(false);
  const reconnectTimerRef = useRef<number | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const connectRef = useRef<() => Promise<void>>(undefined);

  const [pc, setPC] = useState<RTCPeerConnection | null>(null);
  const [peerId, setPeerId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting');
  const [playBlocked, setPlayBlocked] = useState(false);

  useEffect(() => {
    if (typeof RTCPeerConnection === 'undefined') {
      queueMicrotask(() => setError('WebRTC not supported in this browser'));
      return;
    }

    cancelledRef.current = false;

    const scheduleReconnect = () => {
      if (reconnectTimerRef.current) return;
      const attempt = reconnectAttemptsRef.current;
      const delay = Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_MAX_MS);
      reconnectAttemptsRef.current = attempt + 1;
      reconnectTimerRef.current = window.setTimeout(() => {
        reconnectTimerRef.current = null;
        if (!cancelledRef.current) connectRef.current?.();
      }, delay);
    };

    const callbacks: PeerConnectionCallbacks = {
      onConnected: () => {
        reconnectAttemptsRef.current = 0;
        setConnectionState('connected');
      },
      onOffline: () => setConnectionState('offline'),
      onStream: (stream) => {
        const video = videoRef.current;
        if (!video) return;
        video.srcObject = stream;
        video.play().catch((error_: DOMException) => {
          console.warn(`WebRTC [${streamId}]: play() failed:`, error_.name, error_.message);
          if (video.paused) setPlayBlocked(true);
        });
      },
    };

    const connect = async () => {
      setConnectionState('connecting');

      if (pcRef.current) {
        pcRef.current.close();
        pcRef.current = null;
      }

      const peerConnection = new RTCPeerConnection({ iceServers: [] });
      pcRef.current = peerConnection;
      setPC(peerConnection);

      attachPeerHandlers(peerConnection, cancelledRef, callbacks, scheduleReconnect);

      peerConnection.addTransceiver('video', { direction: 'recvonly' });
      peerConnection.addTransceiver('audio', { direction: 'recvonly' });

      try {
        const extractedPeerId = await performSignaling(peerConnection, streamId, cancelledRef);
        setPeerId(extractedPeerId);
      } catch (error_) {
        // Loud first attempt, quiet retries: avoids spamming during transient 404s.
        const log = reconnectAttemptsRef.current === 0 ? console.error : console.debug;
        log(`WebRTC [${streamId}]: connection failed`, error_);
        if (!cancelledRef.current) {
          setConnectionState('offline');
          scheduleReconnect();
        }
      }
    };

    connectRef.current = connect;
    connect();

    const videoElement = videoRef.current;

    return () => {
      cancelledRef.current = true;
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
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

  const handleClickToPlay = () => {
    const video = videoRef.current;
    if (video) {
      video.play().then(() => setPlayBlocked(false)).catch(console.error);
    }
  };

  if (error) {
    return (
      <div className={`relative flex items-center justify-center ${className}`} style={{ background: '#000' }}>
        <span className="text-danger text-sm">{error}</span>
      </div>
    );
  }

  const isOffline = connectionState === 'offline';
  const isConnecting = connectionState === 'connecting';

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
          <span className="text-fg-subtle text-sm">Stream offline</span>
        </div>
      )}
      {isConnecting && (
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-fg-subtle text-sm">Connecting...</span>
        </div>
      )}
      {showStats && connectionState === 'connected' && (
        <StatsOverlay pc={pc} videoRef={videoRef} streamId={streamId} peerId={peerId} />
      )}
      {playBlocked && (
        <div
          className="absolute inset-0 flex items-center justify-center cursor-pointer bg-surface-overlay"
          onClick={handleClickToPlay}
        >
          <div className="text-fg-inverse text-center">
            <PlayIcon className="w-16 h-16 mx-auto" />
            <span className="text-sm mt-2 block">Click to play</span>
          </div>
        </div>
      )}
    </div>
  );
}

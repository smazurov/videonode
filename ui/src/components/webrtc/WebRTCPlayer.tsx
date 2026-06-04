import { useEffect, useRef, useState } from 'react';
import { ArrowPathIcon, PlayIcon, SignalSlashIcon } from '@heroicons/react/24/outline';
import { whepConnect, whepTeardown } from '../../lib/whep';
import { StatsOverlay } from './StatsOverlay';
import { cn } from '../../utils';

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
    // Server assigns MSIDs audio-0, audio-1, ... in canvas device order. Default
    // to playing only audio-0 so the audible track doesn't depend on which
    // ontrack callback fires first; consumers can re-enable others by toggling
    // track.enabled (no renegotiation needed).
    if (e.track.kind === 'audio' && e.track.id !== 'audio-0') {
      e.track.enabled = false;
    }
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
  cancelledRef: React.RefObject<boolean>,
  sessionIdRef: React.RefObject<string | null>
): Promise<string | null> {
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await waitForIceGathering(pc, ICE_GATHER_TIMEOUT_MS);

  if (cancelledRef.current) return null;

  const { answer, sessionId } = await whepConnect(streamId, pc.localDescription!.sdp);
  // Record the session immediately so cleanup can DELETE it even if we bail
  // before applying the answer below.
  sessionIdRef.current = sessionId;
  if (cancelledRef.current) return null;

  await pc.setRemoteDescription({ type: 'answer', sdp: answer });
  return sessionId;
}

export function WebRTCPlayer({ streamId, className = '', muted = true, showStats = false }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const sessionIdRef = useRef<string | null>(null);
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

    // Blank the <video> so the last decoded frame doesn't linger under the
    // offline/connecting overlays. Called on every transition into a no-video
    // state; ontrack repopulates srcObject on the next successful connect.
    const clearFrame = () => {
      if (videoRef.current) videoRef.current.srcObject = null;
    };

    // Best-effort DELETE of the current WHEP session, then forget it.
    const teardownSession = () => {
      const sessionId = sessionIdRef.current;
      if (sessionId) {
        sessionIdRef.current = null;
        void whepTeardown(streamId, sessionId);
      }
    };

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
      onOffline: () => {
        clearFrame();
        setConnectionState('offline');
      },
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
      clearFrame();

      if (pcRef.current) {
        pcRef.current.close();
        pcRef.current = null;
      }
      teardownSession();

      const peerConnection = new RTCPeerConnection({ iceServers: [] });
      pcRef.current = peerConnection;
      setPC(peerConnection);

      attachPeerHandlers(peerConnection, cancelledRef, callbacks, scheduleReconnect);

      peerConnection.addTransceiver('video', { direction: 'recvonly' });
      peerConnection.addTransceiver('audio', { direction: 'recvonly' });

      try {
        const sessionId = await performSignaling(peerConnection, streamId, cancelledRef, sessionIdRef);
        setPeerId(sessionId);
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
      teardownSession();
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
      <div className={cn('relative flex items-center justify-center bg-black', className)}>
        <span className="text-danger text-sm">{error}</span>
      </div>
    );
  }

  const isOffline = connectionState === 'offline';
  const isConnecting = connectionState === 'connecting';

  return (
    <div className={cn('relative bg-black', className)}>
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted={muted}
        className="w-full h-full object-contain"
      />
      {isOffline && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black text-fg-inverse">
          <SignalSlashIcon className="w-14 h-14 text-danger" />
          <span className="text-lg font-semibold uppercase tracking-wider">Stream offline</span>
        </div>
      )}
      {isConnecting && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black text-fg-inverse">
          <ArrowPathIcon className="w-12 h-12 animate-spin text-fg-subtle" />
          <span className="text-sm font-medium uppercase tracking-wider text-fg-subtle">Connecting…</span>
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

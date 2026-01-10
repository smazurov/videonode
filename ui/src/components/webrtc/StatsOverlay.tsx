import { useEffect, useState, useRef, useCallback } from 'react';
import type { WebRTCStats, QualityScore, StatsSample } from './types';
import { HistoryBar, FramesHistoryBar } from './HistoryBars';

const HISTORY_SIZE = 30;
const POLL_INTERVAL_MS = 1000;
const POOR_QUALITY_THRESHOLD = 2;

interface StatsOverlayProps {
  readonly pc: RTCPeerConnection | null;
  readonly videoRef: React.RefObject<HTMLVideoElement | null>;
  readonly streamId: string;
  readonly onClose?: () => void;
}

interface InboundRtpReport extends RTCInboundRtpStreamStats {
  jitterBufferDelay?: number;
  jitterBufferEmittedCount?: number;
  frameWidth?: number;
  frameHeight?: number;
  framesDropped?: number;
}

function calculateQuality(stats: WebRTCStats, packetsStalled: boolean): QualityScore {
  const { packetsLost, packetsReceived } = stats;
  if (packetsStalled) return 'poor';
  if (packetsReceived === 0) return 'unknown';

  const lossRate = (packetsLost / packetsReceived) * 100;
  if (lossRate < 0.1) return 'excellent';
  if (lossRate < 1) return 'good';
  if (lossRate < 3) return 'fair';
  return 'poor';
}

function formatBitrate(bps: number): string {
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(1)} Mbps`;
  if (bps >= 1_000) return `${(bps / 1_000).toFixed(0)} Kbps`;
  return `${bps} bps`;
}

function formatBytesPerSec(bps: number): string {
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(1)} MB/s`;
  if (bps >= 1_000) return `${(bps / 1_000).toFixed(0)} KB/s`;
  return `${bps} B/s`;
}

function createEmptyStats(): WebRTCStats {
  return {
    resolution: null,
    fps: null,
    framesDecoded: 0,
    framesDropped: 0,
    totalVideoFrames: 0,
    droppedVideoFrames: 0,
    videoCodec: null,
    bitrate: 0,
    bytesReceived: 0,
    packetsReceived: 0,
    packetsLost: 0,
    jitter: 0,
    jitterBufferDelay: 0,
    jitterBufferEmittedCount: 0,
    rtt: null,
    audioCodec: null,
    audioPacketsLost: 0,
  };
}

function buildCodecMap(rtcStats: RTCStatsReport): Map<string, string> {
  const codecMap = new Map<string, string>();
  for (const report of rtcStats.values()) {
    const statsReport = report as RTCStats;
    if (statsReport.type === 'codec') {
      const codec = report as unknown as { id: string; mimeType: string };
      codecMap.set(codec.id, codec.mimeType);
    }
  }
  return codecMap;
}

function processVideoRtp(r: InboundRtpReport, stats: WebRTCStats, codecMap: Map<string, string>): void {
  stats.framesDecoded = r.framesDecoded ?? 0;
  stats.fps = r.framesPerSecond ?? null;
  stats.packetsReceived = r.packetsReceived ?? 0;
  stats.packetsLost = r.packetsLost ?? 0;
  stats.jitter = r.jitter ?? 0;
  stats.bytesReceived = r.bytesReceived ?? 0;
  stats.jitterBufferDelay = r.jitterBufferDelay ?? 0;
  stats.jitterBufferEmittedCount = r.jitterBufferEmittedCount ?? 0;
  stats.framesDropped = r.framesDropped ?? 0;

  if (r.frameWidth && r.frameHeight) {
    stats.resolution = { width: r.frameWidth, height: r.frameHeight };
  }

  if (r.codecId) {
    const mimeType = codecMap.get(r.codecId);
    if (mimeType) {
      stats.videoCodec = mimeType.split('/')[1]?.toUpperCase() ?? 'unknown';
    }
  }
}

function processAudioRtp(r: InboundRtpReport, stats: WebRTCStats, codecMap: Map<string, string>): void {
  stats.audioPacketsLost = r.packetsLost ?? 0;
  if (r.codecId) {
    const mimeType = codecMap.get(r.codecId);
    if (mimeType) {
      stats.audioCodec = mimeType.split('/')[1] ?? 'unknown';
    }
  }
}

function processCandidatePair(pair: RTCIceCandidatePairStats, stats: WebRTCStats): void {
  if (pair.nominated || pair.state === 'succeeded' || pair.state === 'in-progress') {
    if (pair.currentRoundTripTime && pair.currentRoundTripTime > 0) {
      stats.rtt = pair.currentRoundTripTime * 1000;
    }
    if (pair.availableIncomingBitrate) {
      stats.bitrate = pair.availableIncomingBitrate;
    }
  }
}

function processRtcReports(rtcStats: RTCStatsReport, stats: WebRTCStats, codecMap: Map<string, string>): void {
  for (const report of rtcStats.values()) {
    const statsReport = report as RTCStats;

    if (statsReport.type === 'inbound-rtp') {
      const r = report as InboundRtpReport;
      if (r.kind === 'video') {
        processVideoRtp(r, stats, codecMap);
      } else if (r.kind === 'audio') {
        processAudioRtp(r, stats, codecMap);
      }
    } else if (statsReport.type === 'candidate-pair') {
      processCandidatePair(report as RTCIceCandidatePairStats, stats);
    }
  }
}

function getVideoPlaybackQuality(video: HTMLVideoElement | null, stats: WebRTCStats): void {
  if (video && 'getVideoPlaybackQuality' in video) {
    const quality = video.getVideoPlaybackQuality();
    stats.totalVideoFrames = quality.totalVideoFrames;
    stats.droppedVideoFrames = quality.droppedVideoFrames;
  }
}

export function StatsOverlay({ pc, videoRef, streamId, onClose }: StatsOverlayProps) {
  const [stats, setStats] = useState<WebRTCStats | null>(null);
  const [history, setHistory] = useState<StatsSample[]>([]);
  const [showWarning, setShowWarning] = useState(false);
  const lastBytesRef = useRef(0);
  const lastTimestampRef = useRef(0);
  const lastPacketsRef = useRef(0);
  const lastFramesDecodedRef = useRef(0);
  const poorQualityCountRef = useRef(0);

  const pollStats = useCallback(async () => {
    if (!pc || pc.connectionState !== 'connected') return;

    try {
      const rtcStats = await pc.getStats();
      const newStats = createEmptyStats();
      const codecMap = buildCodecMap(rtcStats);

      processRtcReports(rtcStats, newStats, codecMap);

      // Calculate bytes per second
      const now = Date.now();
      let bytesPerSec = 0;
      if (lastTimestampRef.current > 0) {
        const elapsed = (now - lastTimestampRef.current) / 1000;
        if (elapsed > 0) {
          bytesPerSec = (newStats.bytesReceived - lastBytesRef.current) / elapsed;
        }
      }
      lastBytesRef.current = newStats.bytesReceived;
      lastTimestampRef.current = now;

      const framesDecodedDelta = newStats.framesDecoded - lastFramesDecodedRef.current;
      lastFramesDecodedRef.current = newStats.framesDecoded;

      // Estimate bitrate from bytes if not available
      if (newStats.bitrate === 0 && bytesPerSec > 0) {
        newStats.bitrate = bytesPerSec * 8;
      }

      getVideoPlaybackQuality(videoRef.current, newStats);

      const packetsStalled = newStats.packetsReceived > 0 && newStats.packetsReceived === lastPacketsRef.current;
      lastPacketsRef.current = newStats.packetsReceived;

      const newQuality = calculateQuality(newStats, packetsStalled);
      setStats(newStats);

      setHistory(prev => {
        const sample: StatsSample = {
          timestamp: now,
          bitrate: newStats.bitrate,
          bytesPerSec,
          bufferHealth: newStats.jitter * 1000,
          framesDecodedDelta,
          quality: newQuality,
        };
        const updated = [...prev, sample];
        return updated.length > HISTORY_SIZE ? updated.slice(-HISTORY_SIZE) : updated;
      });

      if (newQuality === 'poor') {
        poorQualityCountRef.current++;
        if (poorQualityCountRef.current >= POOR_QUALITY_THRESHOLD) {
          setShowWarning(true);
        }
      } else {
        poorQualityCountRef.current = 0;
        setShowWarning(false);
      }
    } catch {
      // Connection closed
    }
  }, [pc, videoRef]);

  useEffect(() => {
    if (!pc) return;

    queueMicrotask(() => void pollStats());
    const interval = setInterval(() => void pollStats(), POLL_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [pc, pollStats]);

  if (!stats) {
    return (
      <div className="absolute top-2 left-2 bg-black/80 text-white px-3 py-2 rounded text-xs font-mono">
        Connecting...
      </div>
    );
  }

  const lossPercent = stats.packetsReceived > 0
    ? ((stats.packetsLost / stats.packetsReceived) * 100).toFixed(2)
    : '0.00';

  return (
    <>
      {showWarning && (
        <div className="absolute top-0 left-0 right-0 bg-yellow-600/90 text-white text-center py-1 text-xs font-medium">
          Poor connection quality
        </div>
      )}

      <div className="absolute top-2 left-2 bg-black/85 text-white px-3 py-2 rounded text-xs font-mono min-w-[280px]">
        {onClose && (
          <button
            onClick={onClose}
            className="absolute top-1 right-1 text-gray-400 hover:text-white px-1"
            aria-label="Close stats"
          >
            [X]
          </button>
        )}

        <table className="w-full">
          <tbody className="[&_td]:py-0.5 [&_td:first-child]:text-gray-400 [&_td:first-child]:pr-4">
            <tr>
              <td>Stream ID</td>
              <td className="text-gray-100">{streamId}</td>
            </tr>
            <tr>
              <td>Resolution</td>
              <td>
                {stats.resolution
                  ? `${stats.resolution.width}x${stats.resolution.height}@${stats.fps?.toFixed(0) ?? '--'}`
                  : '--'}
              </td>
            </tr>
            <tr>
              <td>Frames (RTC)</td>
              <td>
                <FramesHistoryBar samples={history} />
                {stats.framesDecoded.toLocaleString()} decoded / {stats.framesDropped} dropped
              </td>
            </tr>
            <tr>
              <td>Frames (video)</td>
              <td>
                {stats.totalVideoFrames.toLocaleString()} total / {stats.droppedVideoFrames} dropped
              </td>
            </tr>
            <tr>
              <td>Codecs</td>
              <td>
                {stats.videoCodec ?? '--'}
                {stats.audioCodec && ` / ${stats.audioCodec}`}
              </td>
            </tr>
            <tr>
              <td>Connection Speed</td>
              <td>
                <HistoryBar
                  samples={history}
                  getValue={s => s.bitrate}
                  maxValue={10_000_000}
                  label={formatBitrate(stats.bitrate)}
                />
              </td>
            </tr>
            <tr>
              <td>Network Activity</td>
              <td>
                <HistoryBar
                  samples={history}
                  getValue={s => s.bytesPerSec}
                  maxValue={2_000_000}
                  label={formatBytesPerSec(history[history.length - 1]?.bytesPerSec ?? 0)}
                />
              </td>
            </tr>
            <tr>
              <td>Jitter Buffer</td>
              <td>
                {stats.jitterBufferEmittedCount > 0
                  ? `${((stats.jitterBufferDelay / stats.jitterBufferEmittedCount) * 1000).toFixed(0)} ms`
                  : '-- ms'
                } (jitter: {(stats.jitter * 1000).toFixed(1)} ms)
              </td>
            </tr>
            <tr>
              <td>Packets</td>
              <td>
                {stats.packetsReceived.toLocaleString()} received
                {stats.packetsLost > 0 && (
                  <span className="text-red-400"> / {stats.packetsLost} lost ({lossPercent}%)</span>
                )}
              </td>
            </tr>
            {stats.rtt !== null && (
              <tr>
                <td>RTT</td>
                <td>{stats.rtt.toFixed(1)} ms</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}

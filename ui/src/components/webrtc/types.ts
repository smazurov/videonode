export interface WebRTCStats {
  resolution: { width: number; height: number } | null;
  fps: number | null;
  framesDecoded: number;
  // WebRTC stats (inbound-rtp)
  framesDropped: number;
  // Video element playback quality (getVideoPlaybackQuality)
  totalVideoFrames: number;
  droppedVideoFrames: number;
  corruptedVideoFrames: number;
  videoCodec: string | null;
  bitrate: number;
  bytesReceived: number;
  packetsReceived: number;
  packetsLost: number;
  jitter: number;
  jitterBufferDelay: number;
  jitterBufferEmittedCount: number;
  rtt: number | null;
  audioCodec: string | null;
  audioPacketsLost: number;
}

export type QualityScore = 'excellent' | 'good' | 'fair' | 'poor' | 'unknown';

export interface StatsSample {
  timestamp: number;
  bitrate: number;
  bytesPerSec: number;
  bufferHealth: number;
  framesDecodedDelta: number;
  quality: QualityScore;
}

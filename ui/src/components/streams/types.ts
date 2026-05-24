// Local type stubs for the U13 stream-form shape. These mirror the
// canonical Go types from /home/stepan/.claude/plans/plan-a-full-rewrite-linked-gray.md
// (Stream, EncoderConfig, AudioConfig, PublishTarget). They will be
// replaced by `components['schemas']['*']` once `pnpm gen:api` runs
// against the new backend OpenAPI in U1.

export type EncoderCodec = 'h264' | 'h265' | 'av1';
export type RateControl = 'cbr' | 'vbr' | 'cqp';
export type AudioCodec = 'aac' | 'opus';
export type PublishType = 'rtsp' | 'srt' | 'hls';

export interface EncoderConfig {
  codec: EncoderCodec;
  bitrate: string;
  gop?: number;
  rate_control?: RateControl;
  preset?: string;
}

export interface AudioConfig {
  devices: string[];
  codec: AudioCodec;
  bitrate?: string;
  mix_filter?: string;
}

export interface PublishTarget {
  type: PublishType;
  url: string;
}

export interface StreamFormValue {
  stream_id: string;
  upstream: string;
  encoder: EncoderConfig;
  audio: AudioConfig;
  publish: PublishTarget[];
  custom_encoder_args?: string;
}

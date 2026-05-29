// Local type stubs for the stream-form shape. These mirror the canonical
// Go types (Stream, EncoderConfig, AudioConfig). The encoder's output is
// the daemon's local RTSP relay, hardcoded server-side — not a form field.

export type EncoderCodec = 'h264' | 'h265' | 'av1';
export type RateControl = 'cbr' | 'vbr' | 'cqp';
export type AudioCodec = 'aac' | 'opus';

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

export interface StreamFormValue {
  stream_id: string;
  upstream: string;
  encoder: EncoderConfig;
  audio: AudioConfig;
  custom_encoder_args?: string;
}

import { toast } from 'react-hot-toast';
import { clearAuthState } from '../hooks/useAuthStore';

export const API_BASE_URL = `${window.location.protocol}//${window.location.hostname}:8090`;

// Helper function to build full URLs from backend's :port/path format
export function buildStreamURL(partialUrl: string | undefined, protocol: 'http' | 'rtsp' | 'srt' = 'http'): string | undefined {
  if (!partialUrl) return undefined;

  // Backend returns format like ":8889/stream-001" or ":8890?streamid=read:stream-001"
  if (partialUrl.startsWith(':')) {
    const fullUrl = `${protocol}://${window.location.hostname}${partialUrl}`;

    // Add 50ms latency for SRT streams
    if (protocol === 'srt') {
      return `${fullUrl}&latency=50000`;
    }

    return fullUrl;
  }

  // If it's already a full URL, return as-is
  return partialUrl;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

export async function makeApiRequest(
  endpoint: string,
  options: RequestInit = {}
): Promise<Response> {
  const credentials = localStorage.getItem('auth_credentials');
  
  if (!credentials) {
    throw new ApiError(401, 'No credentials found');
  }
  
  const url = `${API_BASE_URL}${endpoint}`;
  
  const response = await fetch(url, {
    ...options,
    headers: {
      'Authorization': `Basic ${credentials}`,
      'Content-Type': 'application/json',
      ...options.headers,
    },
  });
  
  if (!response.ok) {
    // Clear auth state and redirect to login on 401
    if (response.status === 401) {
      toast.error('Session expired. Please log in again.');
      clearAuthState();
    }
    throw new ApiError(response.status, `API request failed: ${response.statusText}`);
  }
  
  return response;
}

export async function apiGet<T>(endpoint: string): Promise<T> {
  const response = await makeApiRequest(endpoint);
  return response.json() as Promise<T>;
}

export async function apiPost<T>(endpoint: string, data?: unknown): Promise<T> {
  const response = await makeApiRequest(endpoint, {
    method: 'POST',
    body: data ? JSON.stringify(data) : null,
  });
  return response.json() as Promise<T>;
}

export async function apiPut<T>(endpoint: string, data?: unknown): Promise<T> {
  const response = await makeApiRequest(endpoint, {
    method: 'PUT',
    body: data ? JSON.stringify(data) : null,
  });
  return response.json() as Promise<T>;
}

export async function apiPatch<T>(endpoint: string, data?: unknown): Promise<T> {
  const response = await makeApiRequest(endpoint, {
    method: 'PATCH',
    body: data ? JSON.stringify(data) : null,
  });
  return response.json() as Promise<T>;
}

export async function apiDelete(endpoint: string): Promise<void> {
  await makeApiRequest(endpoint, {
    method: 'DELETE',
  });
}

export async function testAuth(username: string, password: string): Promise<boolean> {
  const credentials = btoa(`${username}:${password}`);

  try {
    // Use /api/streams which requires auth, unlike /api/health which is public
    const response = await fetch(`${API_BASE_URL}/api/streams`, {
      headers: {
        'Authorization': `Basic ${credentials}`,
        'Content-Type': 'application/json',
      },
    });

    return response.ok;
  } catch (error) {
    console.error("Auth test failed:", error);
    return false;
  }
}

// Stream-related types
export interface StreamData {
  stream_id: string;
  device_id: string;
  codec: string;
  bitrate?: string;
  start_time?: string;
  rtsp_url?: string;
  // Configuration fields for editing
  input_format?: string;
  resolution?: string;
  framerate?: string;
  audio_device?: string;
  custom_ffmpeg_command?: string;
  test_mode?: boolean;
  options?: string[];
  // Metrics fields
  fps?: string;
  dropped_frames?: string;
  duplicate_frames?: string;
}

export interface StreamListData {
  streams: StreamData[];
  count: number;
}

export interface StreamRequestData {
  stream_id: string;
  device_id: string;
  codec: string;
  input_format: string;
  bitrate?: number; // In Mbps
  width?: number;
  height?: number;
  framerate?: number;
  audio_device?: string;
  options?: string[];
  custom_ffmpeg_command?: string;
  test_mode?: boolean;
}

export interface DeviceInfo {
  device_path: string;
  device_name: string;
  device_id: string;
  caps: number;
  capabilities: string[];
}

export interface DeviceData {
  devices: DeviceInfo[];
  count: number;
}

// Stream API functions
export async function getStreams(): Promise<StreamListData> {
  return apiGet<StreamListData>('/api/streams');
}

export async function createStream(request: StreamRequestData): Promise<StreamData> {
  return apiPost<StreamData>('/api/streams', request);
}

export async function updateStream(streamId: string, data: Partial<StreamRequestData>): Promise<StreamData> {
  return apiPatch<StreamData>(`/api/streams/${streamId}`, data);
}

export async function deleteStream(streamId: string): Promise<void> {
  await apiDelete(`/api/streams/${streamId}`);
}

// Device API functions
export async function getDevices(): Promise<DeviceData> {
  return apiGet<DeviceData>('/api/devices');
}

// Device capabilities types
export interface FormatInfo {
  format_name: string;
  original_name: string;
  emulated: boolean;
}

export interface DeviceCapabilitiesData {
  device_path: string;
  formats: FormatInfo[];
}

export interface Resolution {
  width: number;
  height: number;
  type: 'discrete' | 'stepwise' | 'continuous';
  min_width?: number;
  max_width?: number;
  step_width?: number;
  min_height?: number;
  max_height?: number;
  step_height?: number;
}

export interface DeviceResolutionsData {
  resolutions: Resolution[];
}

export interface Framerate {
  numerator: number;
  denominator: number;
  fps: number;
  type: 'discrete' | 'stepwise' | 'continuous';
  min_numerator?: number;
  min_denominator?: number;
  max_numerator?: number;
  max_denominator?: number;
  step_numerator?: number;
  step_denominator?: number;
}

export interface DeviceFrameratesData {
  framerates: Framerate[];
}

// Version info types
export interface VersionInfo {
  version: string;
  git_commit: string;
  build_date: string;
  build_id: string;
  go_version: string;
  compiler: string;
  platform: string;
}

// System API functions
export async function getVersion(): Promise<VersionInfo> {
  return apiGet<VersionInfo>('/api/version');
}

// Device capabilities API functions
export async function getDeviceFormats(deviceId: string): Promise<DeviceCapabilitiesData> {
  return apiGet<DeviceCapabilitiesData>(`/api/devices/${deviceId}/formats`);
}

export async function getDeviceResolutions(deviceId: string, formatName: string): Promise<DeviceResolutionsData> {
  const params = new URLSearchParams({ format_name: formatName });
  return apiGet<DeviceResolutionsData>(`/api/devices/${deviceId}/resolutions?${params}`);
}

export async function getDeviceFramerates(
  deviceId: string, 
  formatName: string, 
  width: number, 
  height: number
): Promise<DeviceFrameratesData> {
  const params = new URLSearchParams({
    format_name: formatName,
    width: width.toString(),
    height: height.toString()
  });
  return apiGet<DeviceFrameratesData>(`/api/devices/${deviceId}/framerates?${params}`);
}

// Health API types and functions
export interface HealthData {
  status: string;
  message: string;
  version?: string;
}

export async function getHealth(): Promise<HealthData> {
  return apiGet<HealthData>('/api/health');
}

// Encoder API types and functions
export interface EncoderInfo {
  type: 'video' | 'audio';
  name: string;
  description: string;
  hwaccel: boolean;
}

export interface EncoderData {
  video_encoders: EncoderInfo[];
  audio_encoders: EncoderInfo[];
  count: number;
}

export async function getEncoders(): Promise<EncoderData> {
  return apiGet<EncoderData>('/api/encoders');
}

// FFmpeg Options API types and functions
export interface FFmpegOption {
  key: string;
  name: string;
  description: string;
  category: string;
  app_default: boolean;
  exclusive_group?: string;
  conflicts_with?: string[];
}

export interface FFmpegOptionsData {
  options: FFmpegOption[];
}

export async function getFFmpegOptions(): Promise<FFmpegOptionsData> {
  return apiGet<FFmpegOptionsData>('/api/options');
}

// SSE Event types
export interface SSEDeviceDiscoveryEvent {
  type: 'device-discovery';
  device_path: string;
  device_name: string;
  device_id: string;
  caps: number;
  capabilities: string[];
  action: 'added' | 'removed' | 'changed';
  timestamp: string;
}

export interface SSEStreamCreatedEvent {
  type: 'stream-created';
  stream: StreamData;
  action: 'created';
  timestamp: string;
}

export interface SSEStreamDeletedEvent {
  type: 'stream-deleted';
  stream_id: string;
  action: 'deleted';
  timestamp: string;
}

export interface SSEStreamUpdatedEvent {
  type: 'stream-updated';
  stream: StreamData;
  action: 'updated' | 'restarted';
  timestamp: string;
}



export interface SSEStreamMetricsEvent {
  type: 'stream-metrics';
  stream_id: string;
  fps?: string;
  dropped_frames?: string;
  duplicate_frames?: string;
}

export type SSEStreamLifecycleEvent = SSEStreamCreatedEvent | SSEStreamUpdatedEvent | SSEStreamDeletedEvent;

// Audio device types
export interface AudioDevice {
  card_number: number;
  card_id: string;
  card_name: string;
  device_number: number;
  device_name: string;
  type: string;
  alsa_device: string;
  supported_rates?: number[];
  min_channels?: number;
  max_channels?: number;
  supported_formats?: string[];
  min_buffer_size?: number;
  max_buffer_size?: number;
  min_period_size?: number;
  max_period_size?: number;
}

export interface AudioDevicesData {
  devices: AudioDevice[];
  count: number;
}

// Audio API functions
export async function getAudioDevices(): Promise<AudioDevicesData> {
  return apiGet<AudioDevicesData>('/api/devices/audio');
}

// FFmpeg command types
export interface FFmpegCommandData {
  stream_id: string;
  command: string;
  is_custom: boolean;
}

// FFmpeg command functions
export async function getFFmpegCommand(streamId: string, encoderOverride?: string): Promise<FFmpegCommandData> {
  const params = encoderOverride ? `?override=${encodeURIComponent(encoderOverride)}` : '';
  return apiGet<FFmpegCommandData>(`/api/streams/${streamId}/ffmpeg${params}`);
}

export async function setFFmpegCommand(streamId: string, command: string): Promise<StreamData> {
  // When setting a custom command, also disable test mode
  return updateStream(streamId, { custom_ffmpeg_command: command, test_mode: false });
}

export async function clearFFmpegCommand(streamId: string): Promise<StreamData> {
  return updateStream(streamId, { custom_ffmpeg_command: "" });
}

export async function toggleTestMode(streamId: string, enabled: boolean): Promise<StreamData> {
  return updateStream(streamId, { test_mode: enabled });
}

export async function restartStream(streamId: string): Promise<void> {
  await makeApiRequest(`/api/streams/${streamId}/restart`, { method: 'POST' });
}

// WebRTC signaling - sends SDP offer, receives SDP answer
// Auth is optional since the backend /api/webrtc endpoint is public
export async function webrtcSignaling(streamId: string, offer: string, signal?: AbortSignal): Promise<string> {
  const credentials = localStorage.getItem('auth_credentials');

  const headers: HeadersInit = {
    'Content-Type': 'application/sdp',
  };

  if (credentials) {
    headers['Authorization'] = `Basic ${credentials}`;
  }

  const response = await fetch(`${API_BASE_URL}/api/webrtc?stream=${encodeURIComponent(streamId)}`, {
    method: 'POST',
    headers,
    body: offer,
    signal: signal ?? null,
  });

  if (!response.ok) {
    if (response.status === 401) {
      toast.error('Session expired. Please log in again.');
      clearAuthState();
    }
    throw new ApiError(response.status, `WebRTC signaling failed: ${response.statusText}`);
  }

  // Response is raw SDP
  return response.text();
}

export type SSEEvent = SSEDeviceDiscoveryEvent | SSEStreamLifecycleEvent | SSEStreamMetricsEvent;
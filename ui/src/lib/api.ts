const API_BASE_URL = "http://localhost:8090";

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
    // Clear invalid credentials on 401
    if (response.status === 401) {
      localStorage.removeItem('auth_credentials');
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

export async function apiDelete(endpoint: string): Promise<void> {
  await makeApiRequest(endpoint, {
    method: 'DELETE',
  });
}

export async function testAuth(username: string, password: string): Promise<boolean> {
  const credentials = btoa(`${username}:${password}`);
  
  try {
    const response = await fetch(`${API_BASE_URL}/api/health`, {
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
  uptime?: number;
  start_time?: string;
  webrtc_url?: string;
  rtsp_url?: string;
}

export interface StreamListData {
  streams: StreamData[];
  count: number;
}

export interface StreamRequestData {
  stream_id: string;
  device_id: string;
  codec: string;
  bitrate?: number;
  width?: number;
  height?: number;
  framerate?: number;
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

export interface CaptureRequestData {
  devicePath: string;
  resolution?: string;
  delay?: number;
}

export interface CaptureData {
  status: string;
  message: string;
  data?: Record<string, string>;
}

// Stream API functions
export async function getStreams(): Promise<StreamListData> {
  return apiGet<StreamListData>('/api/streams');
}

export async function createStream(request: StreamRequestData): Promise<StreamData> {
  return apiPost<StreamData>('/api/streams', request);
}

export async function deleteStream(streamId: string): Promise<void> {
  await apiDelete(`/api/streams/${streamId}`);
}

export async function getStreamStatus(streamId: string): Promise<{
  stream_id: string;
  uptime?: number;
  start_time?: string;
}> {
  return apiGet<{
    stream_id: string;
    uptime?: number;
    start_time?: string;
  }>(`/api/streams/${streamId}/status`);
}

// Device API functions
export async function getDevices(): Promise<DeviceData> {
  return apiGet<DeviceData>('/api/devices');
}

export async function captureFromDevice(request: CaptureRequestData): Promise<CaptureData> {
  return apiPost<CaptureData>('/api/devices/capture', request);
}

// Health API types and functions
export interface HealthData {
  status: string;
  message: string;
  uptime?: number;
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

export interface SSEStreamEvent {
  type: 'stream_started' | 'stream_stopped' | 'stream_error';
  stream_id: string;
  timestamp: string;
}

export interface SSESystemEvent {
  type: 'system-status';
  status: 'online' | 'offline' | 'warning';
  message?: string;
  timestamp: string;
}

export type SSEEvent = SSEDeviceDiscoveryEvent | SSEStreamEvent | SSESystemEvent;
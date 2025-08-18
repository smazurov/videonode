export const COMMON_CODECS = [
  { value: 'h264', label: 'H.264 (Recommended)' },
  { value: 'h265', label: 'H.265/HEVC' },
  { value: 'mjpeg', label: 'MJPEG' },
  { value: 'libx264', label: 'libx264' },
  { value: 'libx265', label: 'libx265' }
] as const;

export const COMMON_RESOLUTIONS = [
  { value: '', label: 'Auto (Device Default)' },
  { value: '3840x2160', label: '4K (3840x2160)' },
  { value: '1920x1080', label: 'Full HD (1920x1080)' },
  { value: '1280x720', label: 'HD (1280x720)' },
  { value: '640x480', label: 'VGA (640x480)' }
] as const;

export const COMMON_FRAMERATES = [
  { value: '', label: 'Auto' },
  { value: '60', label: '60 FPS' },
  { value: '30', label: '30 FPS' },
  { value: '24', label: '24 FPS' },
  { value: '15', label: '15 FPS' }
] as const;

export const COMMON_BITRATES = [
  { value: '', label: 'Auto' },
  { value: '8000', label: '8 Mbps (High Quality)' },
  { value: '4000', label: '4 Mbps (Medium Quality)' },
  { value: '2000', label: '2 Mbps (Standard)' },
  { value: '1000', label: '1 Mbps (Low Bandwidth)' }
] as const;

export const RESOLUTION_LABELS: Record<string, string> = {
  '3840x2160': '4K',
  '2560x1440': '1440p',
  '1920x1080': 'Full HD',
  '1280x720': 'HD',
  '960x540': 'qHD',
  '640x480': 'VGA',
  '640x360': '360p',
} as const;

export const COMMON_RESOLUTION_CONFIGS = [
  { width: 3840, height: 2160 }, // 4K
  { width: 2560, height: 1440 }, // 1440p
  { width: 1920, height: 1080 }, // 1080p
  { width: 1280, height: 720 },  // 720p
  { width: 960, height: 540 },   // qHD
  { width: 640, height: 480 },   // VGA
  { width: 640, height: 360 },   // 360p
] as const;
import { useReducer, useCallback, useEffect } from 'react';
import { useStreamStore } from './useStreamStore';
import { useDeviceFormats, useDeviceResolutions, useDeviceFramerates } from './useDeviceCapabilities';
import { StreamRequestData, StreamData, getFFmpegOptions, Resolution } from '../lib/api';
import { RESOLUTION_LABELS } from '../components/StreamCreation/constants';

// Helper to filter resolutions to common ones
function filterToCommonResolutions(resolutions: Resolution[]): Resolution[] {
  const commonResolutions = resolutions.filter(res => {
    const resString = `${res.width}x${res.height}`;
    return resString in RESOLUTION_LABELS;
  });
  
  // If no common resolutions found, return all
  return commonResolutions.length > 0 ? commonResolutions : resolutions;
}

// State types
export interface StreamFormState {
  // Form values
  streamId: string;
  deviceId: string;
  input_format: string;
  width: number;
  height: number;
  framerate: number;
  codec: string;
  bitrate: number | undefined;
  audio_device: string;
  options: string[];
  
  // Original values for PATCH comparison
  originalValues?: {
    codec: string;
    input_format: string;
    width: number;
    height: number;
    framerate: number;
    bitrate: number | undefined;
    audio_device: string;
    options: string[];
  };
  
  // UI state
  status: 'idle' | 'selecting-device' | 'selecting-format' | 'selecting-resolution' | 'selecting-framerate' | 'ready' | 'creating';
  errors: Record<string, string>;
  
  // Validation state
  isValid: boolean;
}

// Action types
export type StreamFormAction =
  | { type: 'SET_STREAM_ID'; streamId: string }
  | { type: 'SELECT_DEVICE'; deviceId: string }
  | { type: 'SELECT_FORMAT'; format: string }
  | { type: 'SELECT_RESOLUTION'; width: number; height: number }
  | { type: 'SELECT_FRAMERATE'; framerate: number }
  | { type: 'SET_CODEC'; codec: string }
  | { type: 'SET_BITRATE'; bitrate: number | undefined }
  | { type: 'SET_AUDIO_DEVICE'; audioDevice: string }
  | { type: 'SET_OPTIONS'; options: string[] }
  | { type: 'SET_ERROR'; field: string; message: string }
  | { type: 'CLEAR_ERROR'; field: string }
  | { type: 'SET_STATUS'; status: StreamFormState['status'] }
  | { type: 'VALIDATE' }
  | { type: 'RESET' }
  | { type: 'CREATING' }
  | { type: 'CREATE_SUCCESS' }
  | { type: 'CREATE_ERROR'; error: string }
  | { type: 'LOAD_INITIAL_DATA'; streamData: StreamData };

// Initial state
const initialState: StreamFormState = {
  streamId: '',
  deviceId: '',
  input_format: '',
  width: 0,
  height: 0,
  framerate: 0,
  codec: 'h264',
  bitrate: 2, // Default to 2 Mbps
  audio_device: '', // Default to no audio
  options: [], // Will be populated with defaults from API
  status: 'idle',
  errors: {},
  isValid: false,
};

// Validation helper
function validateForm(state: StreamFormState): Record<string, string> {
  const errors: Record<string, string> = {};
  
  if (!state.streamId.trim()) {
    errors.streamId = 'Stream ID is required';
  } else if (!/^[\w-]+$/.test(state.streamId)) {
    errors.streamId = 'Stream ID can only contain letters, numbers, dashes, and underscores';
  }
  
  if (!state.deviceId) {
    errors.deviceId = 'Device selection is required';
  }
  
  if (!state.input_format) {
    errors.input_format = 'Format selection is required';
  }
  
  if (!state.codec) {
    errors.codec = 'Codec selection is required';
  }
  
  if (state.bitrate !== undefined && (state.bitrate < 0.1 || state.bitrate > 50)) {
    errors.bitrate = 'Bitrate must be between 0.1 and 50 Mbps';
  }
  
  return errors;
}

// Reducer
function streamFormReducer(state: StreamFormState, action: StreamFormAction): StreamFormState {
  switch (action.type) {
    case 'SET_STREAM_ID': {
      const newState = { ...state, streamId: action.streamId };
      const errors = validateForm(newState);
      return {
        ...newState,
        errors,
        isValid: Object.keys(errors).length === 0,
      };
    }
    
    case 'SELECT_DEVICE': {
      const restErrors: Record<string, string> = Object.fromEntries(
        Object.entries(state.errors).filter(([key]) => key !== 'deviceId')
      );
      return {
        ...state,
        deviceId: action.deviceId,
        input_format: '', // Clear dependent fields
        width: 0,
        height: 0,
        framerate: 0,
        status: 'selecting-device',
        errors: restErrors,
      };
    }
    
    case 'SELECT_FORMAT': {
      const restErrors: Record<string, string> = Object.fromEntries(
        Object.entries(state.errors).filter(([key]) => !['input_format', 'resolution', 'framerate'].includes(key))
      );
      return {
        ...state,
        input_format: action.format,
        width: 0, // Clear dependent fields
        height: 0,
        framerate: 0,
        status: 'selecting-format',
        errors: restErrors,
      };
    }
    
    case 'SELECT_RESOLUTION': {
      const restErrors: Record<string, string> = Object.fromEntries(
        Object.entries(state.errors).filter(([key]) => key !== 'resolution')
      );
      const newState = {
        ...state,
        width: action.width,
        height: action.height,
        framerate: 0, // Clear dependent field
        status: 'selecting-resolution' as const,
        errors: restErrors,
      };
      
      // Validate to update isValid
      const errors = validateForm(newState);
      return {
        ...newState,
        errors,
        isValid: Object.keys(errors).length === 0,
      };
    }
    
    case 'SELECT_FRAMERATE': {
      const restErrors: Record<string, string> = Object.fromEntries(
        Object.entries(state.errors).filter(([key]) => key !== 'framerate')
      );
      const newState = {
        ...state,
        framerate: action.framerate,
        status: 'ready' as const,
        errors: restErrors,
      };
      
      // Validate to update isValid
      const errors = validateForm(newState);
      return {
        ...newState,
        errors,
        isValid: Object.keys(errors).length === 0,
      };
    }
    
    case 'SET_CODEC': {
      const newErrors = { ...state.errors };
      delete newErrors.codec;
      return {
        ...state,
        codec: action.codec,
        errors: newErrors,
      };
    }
    
    case 'SET_BITRATE': {
      const newErrors = { ...state.errors };
      delete newErrors.bitrate;
      return {
        ...state,
        bitrate: action.bitrate,
        errors: newErrors,
      };
    }
    
    case 'SET_AUDIO_DEVICE': {
      const newErrors = { ...state.errors };
      delete newErrors.audio_device;
      return {
        ...state,
        audio_device: action.audioDevice,
        errors: newErrors,
      };
    }
    
    case 'SET_OPTIONS': {
      return {
        ...state,
        options: action.options,
      };
    }
    
    case 'SET_ERROR': {
      return {
        ...state,
        errors: { ...state.errors, [action.field]: action.message },
        isValid: false,
      };
    }
    
    case 'CLEAR_ERROR': {
      const restErrors: Record<string, string> = Object.fromEntries(
        Object.entries(state.errors).filter(([key]) => key !== action.field)
      );
      return {
        ...state,
        errors: restErrors,
      };
    }
    
    case 'SET_STATUS': {
      return {
        ...state,
        status: action.status,
      };
    }
    
    case 'VALIDATE': {
      const errors = validateForm(state);
      return {
        ...state,
        errors,
        isValid: Object.keys(errors).length === 0,
      };
    }
    
    case 'CREATING': {
      return {
        ...state,
        status: 'creating',
      };
    }
    
    case 'CREATE_SUCCESS': {
      return {
        ...state,
        status: 'ready',
      };
    }
    
    case 'CREATE_ERROR': {
      return {
        ...state,
        status: 'ready',
        errors: { ...state.errors, submit: action.error },
      };
    }
    
    case 'RESET': {
      return initialState;
    }
    
    case 'LOAD_INITIAL_DATA': {
      const streamData = action.streamData;
      
      // Parse resolution from string (e.g., "1920x1080")
      let width = 0, height = 0;
      if (streamData.resolution) {
        const parts = streamData.resolution.split('x');
        if (parts.length === 2) {
          width = parseInt(parts[0] || '0', 10) || 0;
          height = parseInt(parts[1] || '0', 10) || 0;
        }
      }
      
      // Parse framerate
      const framerate = streamData.framerate ? parseInt(streamData.framerate, 10) || 0 : 0;
      
      const codec = streamData.codec;
      const format = streamData.input_format || '';
      const bitrate = streamData.bitrate ? parseFloat(streamData.bitrate.replace(/[^\d.]/g, '')) : undefined;
      const audioDevice = streamData.audio_device || '';
      // Default options will be loaded separately
      const options: string[] = [];
      
      return {
        ...state,
        streamId: streamData.stream_id,
        deviceId: streamData.device_id,
        codec,
        input_format: format,
        width,
        height,
        framerate,
        bitrate,
        audio_device: audioDevice,
        options,
        // Store original values for PATCH comparison
        originalValues: {
          codec,
          input_format: format,
          width,
          height,
          framerate,
          bitrate,
          audio_device: audioDevice,
          options,
        },
        status: 'ready',
        isValid: true,
      };
    }
    
    default: {
      action satisfies never;
      return state;
    }
  }
}

// Main hook
export function useStreamCreation(initialData?: StreamData) {
  const [state, dispatch] = useReducer(streamFormReducer, initialState);
  const { createStream, updateStream } = useStreamStore();
  
  // Load initial data if provided
  useEffect(() => {
    if (initialData) {
      dispatch({ type: 'LOAD_INITIAL_DATA', streamData: initialData });
    }
  }, [initialData]);

  // Load default FFmpeg options on mount
  useEffect(() => {
    getFFmpegOptions().then(data => {
      const defaultOptions = data.options
        .filter(opt => opt.app_default)
        .map(opt => opt.key);
      dispatch({ type: 'SET_OPTIONS', options: defaultOptions });
    }).catch(error => {
      console.error('Failed to load FFmpeg options:', error);
      // Use hardcoded defaults as fallback
      dispatch({ type: 'SET_OPTIONS', options: ['thread_queue_1024', 'copyts'] });
    });
  }, []);
  
  // Fetch device capabilities
  const { formats, loading: loadingFormats } = useDeviceFormats(state.deviceId);
  const { resolutions, loading: loadingResolutions } = useDeviceResolutions(state.deviceId, state.input_format);
  const { framerates, loading: loadingFramerates } = useDeviceFramerates(
    state.deviceId,
    state.input_format,
    state.width,
    state.height
  );
  
  // Auto-select first non-emulated format when formats load
  useEffect(() => {
    if (formats.length > 0 && state.deviceId && !state.input_format) {
      const preferred = formats.find(f => !f.emulated) || formats[0];
      if (preferred) {
        dispatch({ type: 'SELECT_FORMAT', format: preferred.format_name });
      }
    }
  }, [formats, state.deviceId, state.input_format]);
  
  // Auto-select best resolution when resolutions load for a new format
  // But only if this is the initial format selection (status is 'selecting-format')
  useEffect(() => {
    if (resolutions.length > 0 && state.input_format && state.width === 0 && state.height === 0 && state.status === 'selecting-format') {
      // Filter to common resolutions first
      const filtered = filterToCommonResolutions(resolutions);
      
      // Sort by total pixels and pick the highest
      const highest = [...filtered].sort((a, b) => 
        (b.width * b.height) - (a.width * a.height)
      )[0];
      
      if (highest) {
        dispatch({ type: 'SELECT_RESOLUTION', width: highest.width, height: highest.height });
      }
    }
  }, [resolutions, state.input_format, state.width, state.height, state.status]);
  
  // Handle framerate when resolution changes or framerates load
  // Only auto-select if this is the initial resolution selection (status is 'selecting-resolution')
  useEffect(() => {
    if (framerates.length > 0 && state.width > 0 && state.height > 0 && state.status === 'selecting-resolution') {
      // Check if current framerate is still valid
      if (state.framerate > 0) {
        const isCurrentFramerateValid = framerates.some(fr => 
          Math.round(fr.fps) === state.framerate
        );
        
        if (!isCurrentFramerateValid) {
          // Current framerate is not valid for new resolution, auto-select highest
          const highest = [...framerates].sort((a, b) => b.fps - a.fps)[0];
          if (highest) {
            dispatch({ type: 'SELECT_FRAMERATE', framerate: Math.round(highest.fps) });
          }
        }
        // If current framerate is valid, keep it (do nothing)
      } else {
        // No framerate selected, auto-select highest
        const highest = [...framerates].sort((a, b) => b.fps - a.fps)[0];
        if (highest) {
          dispatch({ type: 'SELECT_FRAMERATE', framerate: Math.round(highest.fps) });
        }
      }
    }
  }, [framerates, state.width, state.height, state.framerate, state.status]);
  
  // Action creators
  const setStreamId = useCallback((streamId: string) => {
    dispatch({ type: 'SET_STREAM_ID', streamId });
  }, []);
  
  const selectDevice = useCallback((deviceId: string) => {
    dispatch({ type: 'SELECT_DEVICE', deviceId });
  }, []);
  
  const selectFormat = useCallback((format: string) => {
    dispatch({ type: 'SELECT_FORMAT', format });
  }, []);
  
  const selectResolution = useCallback((width: number, height: number) => {
    // Allow selecting auto (0x0)
    if (width === 0 && height === 0) {
      dispatch({ type: 'SELECT_RESOLUTION', width: 0, height: 0 });
      return;
    }
    // Validate that this resolution exists for the current format
    const filteredResolutions = filterToCommonResolutions(resolutions);
    const isValid = filteredResolutions.some(r => r.width === width && r.height === height);
    if (!isValid) {
      console.warn(`Invalid resolution ${width}x${height} for format ${state.input_format}`);
      return;
    }
    dispatch({ type: 'SELECT_RESOLUTION', width, height });
  }, [resolutions, state.input_format]);
  
  const selectFramerate = useCallback((framerate: number) => {
    dispatch({ type: 'SELECT_FRAMERATE', framerate });
  }, []);
  
  const setCodec = useCallback((codec: string) => {
    dispatch({ type: 'SET_CODEC', codec });
  }, []);
  
  const setBitrate = useCallback((bitrate: number | undefined) => {
    dispatch({ type: 'SET_BITRATE', bitrate });
  }, []);
  
  const setAudioDevice = useCallback((audioDevice: string) => {
    dispatch({ type: 'SET_AUDIO_DEVICE', audioDevice });
  }, []);
  
  const setOptions = useCallback((options: string[]) => {
    dispatch({ type: 'SET_OPTIONS', options });
  }, []);
  
  const handleCreateStream = useCallback(async () => {
    if (!state.isValid) {
      dispatch({ type: 'VALIDATE' });
      return;
    }
    
    dispatch({ type: 'CREATING' });
    
    try {
      const request: StreamRequestData = {
        stream_id: state.streamId,
        device_id: state.deviceId,
        codec: state.codec,
        input_format: state.input_format,
        ...(state.width > 0 ? { width: state.width } : {}),
        ...(state.height > 0 ? { height: state.height } : {}),
        ...(state.framerate > 0 ? { framerate: state.framerate } : {}),
        ...(state.bitrate ? { bitrate: state.bitrate } : {}),
        ...(state.audio_device ? { audio_device: state.audio_device } : {}),
        ...(state.options.length > 0 ? { options: state.options } : {}),
      };
      
      await createStream(request);
      dispatch({ type: 'CREATE_SUCCESS' });
      return true;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to create stream';
      dispatch({ type: 'CREATE_ERROR', error: message });
      return false;
    }
  }, [state, createStream]);
  
  const handleUpdateStream = useCallback(async () => {
    if (!state.isValid) {
      dispatch({ type: 'VALIDATE' });
      return false;
    }
    
    dispatch({ type: 'CREATING' });
    
    try {
      const updateData: Record<string, string | number | string[] | undefined> = {};
      const orig = state.originalValues;
      
      if (orig) {
        // Simple field mapping with diff check
        const fields = [
          ['codec', 'codec'],
          ['input_format', 'input_format'],
          ['width', 'width'],
          ['height', 'height'],
          ['framerate', 'framerate'],
          ['bitrate', 'bitrate'],
          ['audio_device', 'audio_device'],
        ] as const;
        
        for (const [stateKey, apiKey] of fields) {
          if (state[stateKey] !== orig[stateKey]) {
            updateData[apiKey] = state[stateKey];
          }
        }
        
        // Handle options array separately
        if (JSON.stringify(state.options) !== JSON.stringify(orig.options)) {
          updateData.options = state.options;
        }
      }
      
      await updateStream(state.streamId, updateData);
      dispatch({ type: 'CREATE_SUCCESS' });
      return true;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update stream';
      dispatch({ type: 'CREATE_ERROR', error: message });
      return false;
    }
  }, [state, updateStream]);
  
  const reset = useCallback(() => {
    dispatch({ type: 'RESET' });
  }, []);
  
  return {
    // State
    state,
    
    // Available options - filter resolutions here
    formats,
    resolutions: filterToCommonResolutions(resolutions),
    framerates,
    
    // Loading states
    loading: {
      formats: loadingFormats,
      resolutions: loadingResolutions,
      framerates: loadingFramerates,
    },
    
    // Actions
    actions: {
      setStreamId,
      selectDevice,
      selectFormat,
      selectResolution,
      selectFramerate,
      setCodec,
      setBitrate,
      setAudioDevice,
      setOptions,
      createStream: handleCreateStream,
      updateStream: handleUpdateStream,
      reset,
    },
  };
}
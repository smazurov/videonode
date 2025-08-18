import { useState, useCallback } from 'react';
import { StreamRequestData } from '../lib/api';

export interface StreamFormErrors {
  stream_id?: string;
  device_id?: string;
  codec?: string;
  bitrate?: string;
  width?: string;
  height?: string;
  framerate?: string;
  input_format?: string;
}

export interface UseStreamFormReturn {
  formData: StreamRequestData;
  formErrors: StreamFormErrors;
  setFormData: React.Dispatch<React.SetStateAction<StreamRequestData>>;
  setFormErrors: React.Dispatch<React.SetStateAction<StreamFormErrors>>;
  updateField: (field: keyof StreamRequestData, value: string | number | undefined) => void;
  updateResolution: (width?: number, height?: number) => void;
  clearFieldError: (field: keyof StreamFormErrors) => void;
  validateForm: () => boolean;
  resetForm: () => void;
}

const initialFormData: StreamRequestData = {
  stream_id: '',
  device_id: '',
  codec: 'h264',
  input_format: ''
};

export function useStreamForm(): UseStreamFormReturn {
  const [formData, setFormData] = useState<StreamRequestData>(initialFormData);
  const [formErrors, setFormErrors] = useState<StreamFormErrors>({});

  const updateField = useCallback((field: keyof StreamRequestData, value: string | number | undefined) => {
    setFormData(prev => ({
      ...prev,
      [field]: value
    }));
    
    // Clear field error when user starts typing
    if (formErrors[field as keyof StreamFormErrors]) {
      setFormErrors(prev => ({
        ...prev,
        [field]: undefined
      }));
    }
  }, [formErrors]);

  const updateResolution = useCallback((width?: number, height?: number) => {
    setFormData(prev => ({
      ...prev,
      ...(width !== undefined ? { width } : {}),
      ...(height !== undefined ? { height } : {})
    }));
  }, []);

  const clearFieldError = useCallback((field: keyof StreamFormErrors) => {
    setFormErrors(prev => ({
      ...prev,
      [field]: undefined
    }));
  }, []);

  const validateForm = useCallback((): boolean => {
    const errors: StreamFormErrors = {};
    
    if (!formData.stream_id.trim()) {
      errors.stream_id = 'Stream ID is required';
    } else if (!/^[\w-]+$/.test(formData.stream_id)) {
      errors.stream_id = 'Stream ID can only contain letters, numbers, dashes, and underscores';
    }
    
    if (!formData.device_id) {
      errors.device_id = 'Device selection is required';
    }
    
    if (!formData.codec) {
      errors.codec = 'Codec selection is required';
    }
    
    if (formData.bitrate && (formData.bitrate < 100 || formData.bitrate > 50000)) {
      errors.bitrate = 'Bitrate must be between 100 and 50000 kbps';
    }
    
    if (formData.width && (formData.width < 160 || formData.width > 7680)) {
      errors.width = 'Width must be between 160 and 7680 pixels';
    }
    
    if (formData.height && (formData.height < 120 || formData.height > 4320)) {
      errors.height = 'Height must be between 120 and 4320 pixels';
    }
    
    if (formData.framerate && (formData.framerate < 1 || formData.framerate > 120)) {
      errors.framerate = 'Framerate must be between 1 and 120 FPS';
    }
    
    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  }, [formData]);

  const resetForm = useCallback(() => {
    setFormData(initialFormData);
    setFormErrors({});
  }, []);

  return {
    formData,
    formErrors,
    setFormData,
    setFormErrors,
    updateField,
    updateResolution,
    clearFieldError,
    validateForm,
    resetForm
  };
}
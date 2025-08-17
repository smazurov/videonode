import { create } from 'zustand';
import { DeviceInfo, DeviceData, getDevices } from '../lib/api';

interface DeviceStore {
  devices: DeviceInfo[];
  loading: boolean;
  error: string | null;
  lastUpdated: Date | null;
  
  // Actions
  setDevices: (deviceData: DeviceData) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  addDevice: (device: DeviceInfo) => void;
  removeDevice: (deviceId: string) => void;
  fetchDevices: () => Promise<void>;
  reset: () => void;
}

const initialState = {
  devices: [],
  loading: false,
  error: null,
  lastUpdated: null,
};

export const useDeviceStore = create<DeviceStore>((set, get) => ({
  ...initialState,
  
  setDevices: (deviceData: DeviceData) => {
    set({
      devices: deviceData.devices,
      loading: false,
      error: null,
      lastUpdated: new Date(),
    });
  },
  
  setLoading: (loading: boolean) => {
    set({ loading });
  },
  
  setError: (error: string | null) => {
    set({ error, loading: false });
  },
  
  addDevice: (device: DeviceInfo) => {
    const { devices } = get();
    console.log('Store: Adding device', device.device_name, 'Current count:', devices.length);
    // Check if device already exists
    const existingIndex = devices.findIndex(d => d.device_id === device.device_id);
    
    if (existingIndex === -1) {
      const newDevices = [...devices, device];
      console.log('Store: Device added, new count:', newDevices.length);
      set({
        devices: newDevices,
        lastUpdated: new Date(),
      });
    } else {
      console.log('Store: Device already exists, not adding');
    }
  },
  
  removeDevice: (deviceId: string) => {
    const { devices } = get();
    const newDevices = devices.filter(d => d.device_id !== deviceId);
    console.log('Store: Removing device', deviceId, 'New count:', newDevices.length);
    set({
      devices: newDevices,
      lastUpdated: new Date(),
    });
  },
  
  fetchDevices: async () => {
    const { setLoading, setDevices, setError } = get();
    
    try {
      setLoading(true);
      setError(null);
      const deviceData = await getDevices();
      setDevices(deviceData);
    } catch (error) {
      console.error('Failed to fetch devices:', error);
      setError(error instanceof Error ? error.message : 'Failed to fetch devices');
    }
  },
  

  
  reset: () => {
    set(initialState);
  },
}));
import { useState, useEffect, useCallback } from "react";
import { Card } from "./Card";
import { Button } from "./Button";
import StatChart from "./StatChart";
import { 
  ChartBarIcon, 
  XMarkIcon
} from "@heroicons/react/24/outline";
import { 
  getHealth, 
  getStreams, 
  getEncoders,
  type HealthData,
  type StreamListData,
  type EncoderData,
  type SSEEvent
} from "../lib/api";
import { useDeviceStore } from "../hooks/useDeviceStore";
import { useSSEManager } from "../hooks/useSSEManager";
import { cn } from "../utils";

interface StatsSidebarProps {
  isOpen: boolean;
  onToggle: () => void;
  className?: string;
}

interface SystemStats {
  health: HealthData | null;
  streams: StreamListData | null;
  encoders: EncoderData | null;
  loading: boolean;
  error: string | null;
}

export function StatsSidebar({ isOpen, onToggle, className }: Readonly<StatsSidebarProps>) {
  const devices = useDeviceStore((state) => state.devices);
  
  const [stats, setStats] = useState<SystemStats>({
    health: null,
    streams: null,
    encoders: null,
    loading: true,
    error: null
  });

  const [, setRecentEvents] = useState<SSEEvent[]>([]);
  const [bandwidthData, setBandwidthData] = useState<{ date: number; upload: number; download: number }[]>([]);
  const [fpsData, setFpsData] = useState<{ date: number; stat: number }[]>([]);
  const [latencyData, setLatencyData] = useState<{ date: number; stat: number }[]>([]);
  const [jitterData, setJitterData] = useState<{ date: number; stat: number }[]>([]);

  // Initialize data with timestamps (120 data points like jetkvmui)
  useEffect(() => {
    const now = Math.floor(Date.now() / 1000);
    const initialBandwidth = Array.from({ length: 120 }, (_, i) => ({
      date: now - (119 - i),
      // eslint-disable-next-line sonarjs/pseudo-random
      upload: Math.floor(Math.random() * 30) + 40,
      // eslint-disable-next-line sonarjs/pseudo-random
      download: Math.floor(Math.random() * 40) + 60
    }));
    const initialFps = Array.from({ length: 120 }, (_, i) => ({
      date: now - (119 - i),
      // eslint-disable-next-line sonarjs/pseudo-random
      stat: Math.floor(Math.random() * 5) + 28
    }));
    const initialLatency = Array.from({ length: 120 }, (_, i) => ({
      date: now - (119 - i),
      // eslint-disable-next-line sonarjs/pseudo-random
      stat: Math.floor(Math.random() * 8) + 12
    }));
    const initialJitter = Array.from({ length: 120 }, (_, i) => ({
      date: now - (119 - i),
      // eslint-disable-next-line sonarjs/pseudo-random
      stat: Math.floor(Math.random() * 4) + 1
    }));

    setBandwidthData(initialBandwidth);
    setFpsData(initialFps);
    setLatencyData(initialLatency);
    setJitterData(initialJitter);
  }, []);

  // Fetch initial data
  const fetchStats = useCallback(async () => {
    try {
      setStats(prev => ({ ...prev, loading: true, error: null }));
      
      const [health, streams, encoders] = await Promise.all([
        getHealth().catch(() => null),
        getStreams().catch(() => null),
        getEncoders().catch(() => null)
      ]);

      // Also fetch devices
      useDeviceStore.getState().fetchDevices();

      setStats({
        health,
        streams,
        encoders,
        loading: false,
        error: null
      });
    } catch (error) {
      setStats(prev => ({
        ...prev,
        loading: false,
        error: error instanceof Error ? error.message : 'Failed to fetch stats'
      }));
    }
  }, []);

  // Load initial stats
  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  // Setup SSE connection
  useSSEManager({
    onStreamEvent: (event) => {
      setRecentEvents(prev => [event, ...prev.slice(0, 9)]); // Keep last 10 events
    },
    onSystemEvent: (event) => {
      setRecentEvents(prev => [event, ...prev.slice(0, 9)]); // Keep last 10 events
    },
  });

  useEffect(() => {
    // Simulate stats updates (every 500ms like jetkvmui)
    const statsInterval = setInterval(() => {
      const now = Math.floor(Date.now() / 1000);
      
      setBandwidthData(prev => {
        // Only add if timestamp is different from last entry
        if (prev.length > 0 && prev[prev.length - 1]?.date === now) {
          return prev;
        }
        // eslint-disable-next-line sonarjs/pseudo-random
        const uploadValue = Math.floor(Math.random() * 30) + 40;
        // eslint-disable-next-line sonarjs/pseudo-random
        const downloadValue = Math.floor(Math.random() * 40) + 60;
        const newData = [...prev, { date: now, upload: uploadValue, download: downloadValue }];
        return newData.slice(-120);
      });
      setFpsData(prev => {
        if (prev.length > 0 && prev[prev.length - 1]?.date === now) {
          return prev;
        }
        // eslint-disable-next-line sonarjs/pseudo-random
        const newValue = Math.floor(Math.random() * 5) + 28;
        const newData = [...prev, { date: now, stat: newValue }];
        return newData.slice(-120);
      });
      setLatencyData(prev => {
        if (prev.length > 0 && prev[prev.length - 1]?.date === now) {
          return prev;
        }
        // eslint-disable-next-line sonarjs/pseudo-random
        const newValue = Math.floor(Math.random() * 8) + 12;
        const newData = [...prev, { date: now, stat: newValue }];
        return newData.slice(-120);
      });
      setJitterData(prev => {
        if (prev.length > 0 && prev[prev.length - 1]?.date === now) {
          return prev;
        }
        // eslint-disable-next-line sonarjs/pseudo-random
        const newValue = Math.floor(Math.random() * 4) + 1;
        const newData = [...prev, { date: now, stat: newValue }];
        return newData.slice(-120);
      });
    }, 500);

    return () => {
      clearInterval(statsInterval);
    };
  }, []);

  if (!isOpen) {
    return null;
  }

  return (
    <div className={cn(
      "fixed top-0 right-0 h-full w-[493px] bg-white dark:bg-gray-900 border-l border-gray-200 dark:border-gray-700 shadow-xl z-40 transform transition-transform duration-300 ease-in-out overflow-y-auto",
      className
    )}>
      {/* Header */}
      <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 p-4 flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <ChartBarIcon className="h-5 w-5 text-gray-600 dark:text-gray-300" />
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
            Stream Statistics
          </h2>
        </div>
        <Button
          theme="blank"
          size="SM"
          onClick={onToggle}
          LeadingIcon={XMarkIcon}
        />
      </div>

      <div className="p-4 space-y-6">
        {(
          <>
            {/* System Overview */}
            <Card>
              <div className="p-4">
                <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
                  System Overview
                </h3>
                <div className="grid grid-cols-2 gap-3">
                  <div className="bg-gray-50 dark:bg-gray-800/50 rounded-lg p-3">
                    <div className="text-xs text-gray-500 dark:text-gray-400">Status</div>
                    <div className="text-sm font-medium text-gray-900 dark:text-white">
                      {stats.health?.status || 'Unknown'}
                    </div>
                  </div>
                  <div className="bg-gray-50 dark:bg-gray-800/50 rounded-lg p-3">
                    <div className="text-xs text-gray-500 dark:text-gray-400">Devices</div>
                    <div className="text-sm font-medium text-gray-900 dark:text-white">
                      {devices.length}
                    </div>
                  </div>
                  <div className="bg-gray-50 dark:bg-gray-800/50 rounded-lg p-3">
                    <div className="text-xs text-gray-500 dark:text-gray-400">Streams</div>
                    <div className="text-sm font-medium text-gray-900 dark:text-white">
                      {stats.streams?.streams?.length || 0}
                    </div>
                  </div>
                  <div className="bg-gray-50 dark:bg-gray-800/50 rounded-lg p-3">
                    <div className="text-xs text-gray-500 dark:text-gray-400">Encoders</div>
                    <div className="text-sm font-medium text-gray-900 dark:text-white">
                      {stats.encoders?.count || 0}
                    </div>
                  </div>
                </div>
              </div>
            </Card>

            {/* Bandwidth Chart */}
            <Card>
              <div className="p-4">
                <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
                  Bandwidth (Mbps)
                </h3>
                <div style={{ height: 120 }}>
                  <StatChart
                    data={bandwidthData}
                    dataKeys={["upload", "download"]}
                    colors={["#10b981", "#3b82f6"]}
                    domain={[0, 100]}
                  />
                </div>
              </div>
            </Card>

            {/* FPS Chart */}
            <Card>
              <div className="p-4">
                <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
                  Frame Rate (FPS)
                </h3>
                <div style={{ height: 120 }}>
                  <StatChart
                    data={fpsData}
                    dataKeys={["stat"]}
                    colors={["#8b5cf6"]}
                    domain={[0, 60]}
                  />
                </div>
              </div>
            </Card>

            {/* Latency Chart */}
            <Card>
              <div className="p-4">
                <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
                  Latency (ms)
                </h3>
                <div style={{ height: 120 }}>
                  <StatChart
                    data={latencyData}
                    dataKeys={["stat"]}
                    colors={["#f59e0b"]}
                    domain={[0, 50]}
                  />
                </div>
              </div>
            </Card>

            {/* Jitter Chart */}
            <Card>
              <div className="p-4">
                <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
                  Jitter (ms)
                </h3>
                <div style={{ height: 120 }}>
                  <StatChart
                    data={jitterData}
                    dataKeys={["stat"]}
                    colors={["#ef4444"]}
                    domain={[0, 10]}
                  />
                </div>
              </div>
            </Card>
          </>
        )}
      </div>
    </div>
  );
}

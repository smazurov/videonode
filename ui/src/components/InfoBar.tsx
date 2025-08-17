import { useState, useEffect, useCallback } from "react";
import { 
  SignalIcon,
  ComputerDesktopIcon,
  VideoCameraIcon,
  CpuChipIcon,

  ExclamationTriangleIcon,
  CheckCircleIcon,
  ClockIcon
} from "@heroicons/react/24/outline";
import { 
  getHealth, 
  getStreams, 
  getEncoders,
  type HealthData,
  type StreamListData,
  type EncoderData
} from "../lib/api";
import { useDeviceStore } from "../hooks/useDeviceStore";
import { useSSEManager } from "../hooks/useSSEManager";

import { cn } from "../utils";

interface InfoBarProps {
  className?: string;
}

interface SystemInfo {
  health: HealthData | null;
  streams: StreamListData | null;
  encoders: EncoderData | null;
  loading: boolean;
  error: string | null;
  lastUpdated: Date | null;
}

type StatusType = 'online' | 'offline' | 'warning' | 'reconnecting';

interface StatusIndicatorProps {
  status: StatusType;
  size?: 'sm' | 'md';
}

function StatusIndicator({ status, size = 'sm' }: Readonly<StatusIndicatorProps>) {
  const sizeClasses = {
    sm: 'w-2 h-2',
    md: 'w-3 h-3'
  } as const;

  const colorClasses = {
    online: 'bg-green-500',
    warning: 'bg-yellow-500', 
    offline: 'bg-red-500',
    reconnecting: 'bg-yellow-500'
  } as const;

  return (
    <div className={cn(
      "rounded-full animate-pulse",
      sizeClasses[size],
      colorClasses[status]
    )} />
  );
}

interface InfoItemProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string | number;
  status?: StatusType;
  subtitle?: string;
  onClick?: () => void;
}

function InfoItem({ icon: Icon, label, value, status, subtitle, onClick }: Readonly<InfoItemProps>) {
  return (
    <div 
      className={cn(
        "flex items-center space-x-2 px-3 py-1.5 rounded-md transition-colors",
        onClick && "cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700"
      )}
      onClick={onClick}
    >
      <div className="flex items-center space-x-1.5">
        <Icon className="w-4 h-4 text-gray-500 dark:text-gray-400" />
        {status && <StatusIndicator status={status} />}
      </div>
      <div className="flex flex-col">
        <div className="flex items-center space-x-1">
          <span className="text-xs text-gray-600 dark:text-gray-300">{label}:</span>
          <span className="text-xs font-medium text-gray-900 dark:text-white">{value}</span>
        </div>
        {subtitle && (
          <span className="text-xs text-gray-500 dark:text-gray-400">{subtitle}</span>
        )}
      </div>
    </div>
  );
}

interface SeparatorProps {
  className?: string;
}

function Separator({ className }: Readonly<SeparatorProps>) {
  return (
    <div className={cn("w-px h-6 bg-gray-300 dark:bg-gray-600", className)} />
  );
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  
  if (days > 0) {
    return `${days}d ${hours}h`;
  } else if (hours > 0) {
    return `${hours}h ${minutes}m`;
  } else {
    return `${minutes}m`;
  }
}

function formatLastUpdated(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSeconds = Math.floor(diffMs / 1000);
  
  if (diffSeconds < 60) {
    return `${diffSeconds}s ago`;
  } else if (diffSeconds < 3600) {
    return `${Math.floor(diffSeconds / 60)}m ago`;
  } else {
    return `${Math.floor(diffSeconds / 3600)}h ago`;
  }
}

export function InfoBar({ className }: Readonly<InfoBarProps>) {
  const devices = useDeviceStore((state) => state.devices);
  
  // Debug: Log when devices change
  useEffect(() => {
    console.log('InfoBar: Devices updated, count:', devices.length);
  }, [devices]);

  const [systemInfo, setSystemInfo] = useState<SystemInfo>({
    health: null,
    streams: null,
    encoders: null,
    loading: true,
    error: null,
    lastUpdated: null
  });

  const [connectionStatus, setConnectionStatus] = useState<'online' | 'offline' | 'warning' | 'reconnecting'>('offline');

  // Fetch system information
  const fetchSystemInfo = useCallback(async (showLoading = false) => {
    try {
      if (showLoading) {
        setSystemInfo(prev => ({ ...prev, loading: true, error: null }));
      }
      
      const [health, streams, encoders] = await Promise.all([
        getHealth().catch(() => null),
        getStreams().catch(() => null),
        getEncoders().catch(() => null)
      ]);

      // Also fetch devices if this is the initial load
      if (showLoading) {
        useDeviceStore.getState().fetchDevices();
      }

      setSystemInfo({
        health,
        streams,
        encoders,
        loading: false,
        error: null,
        lastUpdated: new Date()
      });

      // Update connection status based on health
      if (health) {
        if (health.status === 'ok' || health.status === 'online') {
          setConnectionStatus('online');
        } else {
          setConnectionStatus('warning');
        }
      } else {
        setConnectionStatus('offline');
      }
    } catch (error) {
      setSystemInfo(prev => ({
        ...prev,
        loading: false,
        error: error instanceof Error ? error.message : 'Failed to fetch system info',
        lastUpdated: new Date()
      }));
      setConnectionStatus('offline');
    }
  }, []);

  // Setup SSE connection for real-time updates
  useSSEManager({
    onStreamEvent: () => {
      fetchSystemInfo(false); // Refresh streams when stream events occur
    },
    onSystemEvent: (event) => {
      if (event.type === 'system-status') {
        setConnectionStatus(event.status);
      }
    },
    onConnectionStatusChange: setConnectionStatus,
  });

  useEffect(() => {
    // Initial fetch
    fetchSystemInfo(true);
  }, [fetchSystemInfo]);

  const getSystemStatus = (): StatusType => {
    if (systemInfo.error) return 'offline';
    if (!systemInfo.health) return 'offline';
    return connectionStatus;
  };

  const getAvailableEncodersCount = () => {
    return systemInfo.encoders?.count || 0;
  };

  const getEncoderStatus = (): 'online' | 'offline' | 'warning' => {
    const availableCount = getAvailableEncodersCount();
    if (availableCount === 0) return 'offline';
    return 'online';
  };

  const getRecommendedEncoder = () => {
    // Prefer hardware accelerated encoders
    const hwEncoder = systemInfo.encoders?.video_encoders?.find(e => e.hwaccel);
    if (hwEncoder) {
      return `${hwEncoder.name} (HW)`;
    }
    // Fall back to first software encoder
    const swEncoder = systemInfo.encoders?.video_encoders?.find(e => !e.hwaccel);
    return swEncoder ? swEncoder.name : 'None';
  };

  return (
    <div className={cn(
      "flex items-center justify-between px-4 py-2 bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 text-xs overflow-x-auto",
      className
    )}>
      {/* Left section - Core metrics */}
      <div className="flex items-center space-x-2 md:space-x-4 flex-shrink-0">

        {/* Device Count */}
        <InfoItem
          icon={VideoCameraIcon}
          label="Devices"
          value={devices.length}
          subtitle={`${devices.filter(d => d.capabilities.includes('VIDEO_CAPTURE')).length} capture`}
        />

        <Separator className="hidden md:block" />

        {/* Stream Count */}
        <InfoItem
          icon={SignalIcon}
          label="Streams"
          value={systemInfo.streams?.count || 0}
          subtitle={systemInfo.streams?.count ? `${systemInfo.streams?.streams.length || 0} active` : "None active"}
        />

        <Separator className="hidden lg:block" />

        {/* Encoder Status */}
        <div className="hidden lg:flex">
          <InfoItem
            icon={CpuChipIcon}
            label="Encoders"
            value={getAvailableEncodersCount()}
            status={getEncoderStatus()}
            subtitle={getRecommendedEncoder()}
          />
        </div>

        {/* Show warnings/errors */}
        {systemInfo.error && (
          <>
            <Separator className="hidden sm:block" />
            <div className="flex items-center space-x-1.5 px-2 py-1 bg-red-50 dark:bg-red-900/20 rounded-md">
              <ExclamationTriangleIcon className="w-4 h-4 text-red-500" />
              <span className="text-xs text-red-700 dark:text-red-400 hidden sm:inline">Connection Error</span>
            </div>
          </>
        )}
      </div>

      {/* Right section - User info and system details */}
      <div className="flex items-center space-x-2 md:space-x-4 flex-shrink-0 ml-4">
        {/* Last updated */}
        {systemInfo.lastUpdated && !systemInfo.loading && (
          <>
            <div className="hidden xl:flex items-center space-x-1.5">
              <ClockIcon className="w-4 h-4 text-gray-400" />
              <span className="text-xs text-gray-500 dark:text-gray-400">
                Updated {formatLastUpdated(systemInfo.lastUpdated)}
              </span>
            </div>
            
            <Separator className="hidden xl:block" />
          </>
        )}

        {/* System Uptime */}
        {systemInfo.health?.uptime && (
          <>
            <div className="hidden lg:flex">
              <InfoItem
                icon={CheckCircleIcon}
                label="Uptime"
                value={formatUptime(systemInfo.health.uptime)}
              />
            </div>
            
            <Separator className="hidden lg:block" />
          </>
        )}

        {/* System Status */}
        <InfoItem
          icon={ComputerDesktopIcon}
          label="System"
          value={(() => {
            if (systemInfo.loading) return "Loading...";
            if (connectionStatus === 'reconnecting') return "Reconnecting";
            return systemInfo.health?.status || "Unknown";
          })()}
          status={getSystemStatus()}
          {...(systemInfo.health?.version && { subtitle: `v${systemInfo.health.version}` })}
        />

        {/* Loading indicator */}
        {systemInfo.loading && (
          <div className="flex items-center space-x-2">
            <div className="w-4 h-4 border-2 border-gray-300 border-t-blue-500 rounded-full animate-spin" />
            <span className="text-xs text-gray-500 dark:text-gray-400 hidden sm:inline">Updating...</span>
          </div>
        )}
      </div>
    </div>
  );
}
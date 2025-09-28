import { useState, useEffect, useMemo } from "react";
import { 
  SignalIcon,
  ComputerDesktopIcon,
  VideoCameraIcon,
  ExclamationTriangleIcon,
  ClockIcon
} from "@heroicons/react/24/outline";
import * as Tooltip from "@radix-ui/react-tooltip";
import { 
  getHealth, 
  getEncoders,
  type HealthData,
  type EncoderData
} from "../lib/api";
import { useDeviceStore } from "../hooks/useDeviceStore";
import { useStreamStore } from "../hooks/useStreamStore";
import { useSSEManager } from "../hooks/useSSEManager";
import { useVersion } from "../hooks/useVersion";
import { cn } from "../utils";

interface InfoBarProps {
  className?: string;
}

interface SystemInfo {
  health: HealthData | null;
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
  const streamsMap = useStreamStore((state) => state.streams);
  const streams = useMemo(() => Array.from(streamsMap.values()), [streamsMap]);
  const { version: versionInfo } = useVersion();
  
  // Debug: Log when devices change
  useEffect(() => {
    console.log('InfoBar: Devices updated, count:', devices.length);
  }, [devices]);

  const [systemInfo, setSystemInfo] = useState<SystemInfo>({
    health: null,
    encoders: null,
    loading: true,
    error: null,
    lastUpdated: null
  });

  const [connectionStatus, setConnectionStatus] = useState<'online' | 'offline' | 'warning' | 'reconnecting'>('offline');

  // Fetch system information
  const fetchSystemInfo = async (showLoading = false) => {
    try {
      if (showLoading) {
        setSystemInfo(prev => ({ ...prev, loading: true, error: null }));
      }
      
      const [health, encoders] = await Promise.all([
        getHealth().catch(() => null),
        getEncoders().catch(() => null)
      ]);

      // Also fetch devices and streams if this is the initial load
      if (showLoading) {
        useDeviceStore.getState().fetchDevices();
        useStreamStore.getState().fetchStreams();
      }

      setSystemInfo({
        health,
        encoders,
        loading: false,
        error: null,
        lastUpdated: new Date()
      });
    } catch (error) {
      setSystemInfo(prev => ({
        ...prev,
        loading: false,
        error: error instanceof Error ? error.message : 'Failed to fetch system info',
        lastUpdated: new Date()
      }));
    }
  };

  // Setup SSE connection for real-time updates
  useSSEManager({
    onConnectionStatusChange: setConnectionStatus,
  });

  useEffect(() => {
    // Initial fetch
    fetchSystemInfo(true);
  }, []);







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
        />

        <Separator className="hidden md:block" />

        {/* Stream Count */}
        <InfoItem
          icon={SignalIcon}
          label="Streams"
          value={streams.length}
        />



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

        {/* SSE Connection Status with Version Tooltip */}
        <Tooltip.Provider>
          <Tooltip.Root>
            <Tooltip.Trigger asChild>
              <div>
                <InfoItem
                  icon={ComputerDesktopIcon}
                  label="System"
                  value={(() => {
                    switch (connectionStatus) {
                      case 'online': return "Connected";
                      case 'offline': return "Disconnected";
                      case 'reconnecting': return "Reconnecting";
                      default: return "Unknown";
                    }
                  })()}
                  status={connectionStatus}
                  {...(systemInfo.health?.version && { subtitle: `v${systemInfo.health.version}` })}
                />
              </div>
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Content
                className="z-50 px-3 py-2 text-xs bg-gray-900 text-white rounded-md shadow-lg"
                sideOffset={5}
              >
                {versionInfo && (
                  <div className="space-y-1 font-mono">
                    <div>
                      <span className="text-gray-400">API:</span> {versionInfo.git_commit} • {versionInfo.build_date}
                    </div>
                    <div>
                      <span className="text-gray-400">UI:</span> {
                        typeof __VIDEONODE_UI_VERSION__ !== 'undefined' ? __VIDEONODE_UI_VERSION__ : 'dev'
                      }
                    </div>
                  </div>
                )}
                {!versionInfo && (
                  <div className="text-gray-400">Loading version info...</div>
                )}
                <Tooltip.Arrow className="fill-gray-900" />
              </Tooltip.Content>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>


      </div>
    </div>
  );
}
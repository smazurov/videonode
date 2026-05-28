import { useState, useEffect, useMemo } from "react";
import {
  SignalIcon,
  ComputerDesktopIcon,
  VideoCameraIcon,
  ExclamationTriangleIcon,
  ClockIcon
} from "@heroicons/react/24/outline";
import * as Tooltip from "@radix-ui/react-tooltip";
import type { components } from "../lib/api.generated";
import { api } from "../lib/api";

type HealthData = components["schemas"]["HealthData"];
type EncoderData = components["schemas"]["EncoderData"];
import { useDeviceStore } from "../hooks/useDeviceStore";
import { useStreamStore } from "../hooks/useStreamStore";
import { useSSEManager } from "../hooks/useSSEManager";
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
    online: 'bg-success',
    warning: 'bg-warning',
    offline: 'bg-danger',
    reconnecting: 'bg-warning'
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
        onClick && "cursor-pointer hover:bg-surface-muted"
      )}
      onClick={onClick}
    >
      <div className="flex items-center space-x-1.5">
        <Icon className="w-4 h-4 text-fg-subtle" />
        {status && <StatusIndicator status={status} />}
      </div>
      <div className="flex flex-col">
        <div className="flex items-center space-x-1">
          <span className="text-xs text-fg-muted">{label}:</span>
          <span className="text-xs font-medium text-fg">{value}</span>
        </div>
        {subtitle && (
          <span className="text-xs text-fg-subtle">{subtitle}</span>
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
    <div className={cn("w-px h-6 bg-border", className)} />
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
  const streamsById = useStreamStore((state) => state.streamsById);
  const streams = useMemo(() => Object.values(streamsById), [streamsById]);

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
      
      const [healthResult, encodersResult] = await Promise.all([
        api.GET("/api/health").catch(() => null),
        api.GET("/api/encoders").catch(() => null),
      ]);
      const health = healthResult?.data ?? null;
      const encoders = encodersResult?.data ?? null;

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
      "flex items-center justify-between px-4 py-2 bg-surface-raised border-t border-border text-xs overflow-x-auto",
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
            <div className="flex items-center space-x-1.5 px-2 py-1 bg-danger-soft rounded-md">
              <ExclamationTriangleIcon className="w-4 h-4 text-danger" />
              <span className="text-xs text-danger-soft-fg hidden sm:inline">Connection Error</span>
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
              <ClockIcon className="w-4 h-4 text-fg-subtle" />
              <span className="text-xs text-fg-subtle">
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
                />
              </div>
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Content
                className="z-50 px-3 py-2 text-xs bg-fg text-fg-inverse rounded-md shadow-lg"
                sideOffset={5}
              >
                <div className="space-y-1 font-mono">
                  <div>
                    <span className="text-fg-subtle">UI:&nbsp;</span> {
                      typeof __VIDEONODE_UI_VERSION__ !== 'undefined' ? __VIDEONODE_UI_VERSION__ : 'dev'
                    }
                  </div>
                </div>
                <Tooltip.Arrow className="fill-fg" />
              </Tooltip.Content>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>


      </div>
    </div>
  );
}
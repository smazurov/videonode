import { useState, useEffect, useMemo } from "react";
import {
  SignalIcon,
  ComputerDesktopIcon,
  VideoCameraIcon,
  ExclamationTriangleIcon,
  ClockIcon,
  CpuChipIcon,
  CircleStackIcon,
  Square3Stack3DIcon
} from "@heroicons/react/24/outline";
import * as Tooltip from "@radix-ui/react-tooltip";
import { Link } from "react-router-dom";
import type { components } from "../lib/api.generated";
import { api } from "../lib/api";

type HealthData = components["schemas"]["HealthData"];
type EncoderData = components["schemas"]["EncoderData"];
import { useDeviceStore } from "../hooks/useDeviceStore";
import { useStreamStore } from "../hooks/useStreamStore";
import { useSSEManager } from "../hooks/useSSEManager";
import { useSystemStats } from "../hooks/useSystemStats";
import { formatUptime } from "../lib/formatUptime";
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

const CONNECTION_STATUS_LABELS: Record<'online' | 'offline' | 'warning' | 'reconnecting', string> = {
  online: "Connected",
  offline: "Disconnected",
  warning: "Warning",
  reconnecting: "Reconnecting",
};

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
  valueClassName?: string;
  onClick?: () => void;
}

function InfoItem({ icon: Icon, label, value, status, subtitle, valueClassName, onClick }: Readonly<InfoItemProps>) {
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
          <span className={cn("text-xs font-medium text-fg", valueClassName)}>{value}</span>
        </div>
        {subtitle && (
          <span className="text-xs text-fg-subtle">{subtitle}</span>
        )}
      </div>
    </div>
  );
}

interface StatProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string | number;
  valueWidth?: string;
  iconClassName?: string;
}

// Compact single-line stat for the resource summary. Value is monospace
// with a reserved min-width so a ticking uptime / changing CPU never
// nudges its neighbours; slack trails the number, not the label.
function Stat({ icon: Icon, label, value, valueWidth, iconClassName }: Readonly<StatProps>) {
  return (
    <span className="flex items-center gap-x-1 whitespace-nowrap">
      <Icon className={cn("w-4 h-4 text-fg-subtle shrink-0", iconClassName)} />
      {/* Label is sans, value is mono — align on the shared text baseline so
          the monospace value doesn't sit lower than the label. */}
      <span className="flex items-baseline gap-x-1">
        <span className="text-fg-muted">{label}:</span>
        <span className={cn("font-mono tabular-nums font-medium text-fg text-left inline-block", valueWidth)}>
          {value}
        </span>
      </span>
    </span>
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

function formatRSS(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${bytes} B`;
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

  // Daemon-wide resource summary (uptime + combined CPU/memory of the
  // whole pipeline, the daemon process included).
  const { stats: systemStats } = useSystemStats();

  // Local clock so the uptime label advances every second between the
  // 2s stat polls.
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

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
        {/* Pipeline-wide resource summary: uptime + combined CPU/memory */}
        {systemStats && (
          <>
            <div className="flex items-center gap-x-3 text-xs">
              <Stat icon={ClockIcon} label="Uptime" valueWidth="min-w-[2.75rem]"
                value={formatUptime(systemStats.started_at_us, now) ?? '—'} />
              <Stat icon={CpuChipIcon} label="CPU" valueWidth="min-w-[2.5rem]"
                value={`${systemStats.cpu_percent.toFixed(1)}%`} />
              <Stat icon={CircleStackIcon} label="Mem" valueWidth="min-w-[3rem]"
                value={formatRSS(systemStats.rss_bytes)} />
              {systemStats.error_count > 0 ? (
                <Tooltip.Provider>
                  <Tooltip.Root>
                    <Tooltip.Trigger asChild>
                      <Link
                        to="/logs"
                        aria-label={`${systemStats.error_count} pipeline error(s)`}
                        className="inline-flex items-center hover:opacity-80"
                      >
                        <Stat
                          icon={ExclamationTriangleIcon}
                          iconClassName="text-danger animate-pulse"
                          label="Procs"
                          valueWidth="min-w-[1rem]"
                          value={systemStats.process_count}
                        />
                      </Link>
                    </Tooltip.Trigger>
                    <Tooltip.Portal>
                      <Tooltip.Content
                        className="z-50 px-3 py-2 text-xs bg-surface-raised text-fg border border-border rounded-md shadow-lg max-w-md"
                        sideOffset={5}
                      >
                        <div className="space-y-1 font-mono">
                          {(systemStats.errors ?? []).map((e) => (
                            <div key={e.id} className="break-all">
                              <span className="text-danger">{e.id}</span>
                              {e.message ? <span className="text-fg-muted">: {e.message}</span> : null}
                            </div>
                          ))}
                        </div>
                        <Tooltip.Arrow className="fill-surface-raised" />
                      </Tooltip.Content>
                    </Tooltip.Portal>
                  </Tooltip.Root>
                </Tooltip.Provider>
              ) : (
                <Stat icon={Square3Stack3DIcon} label="Procs" valueWidth="min-w-[1rem]"
                  value={systemStats.process_count} />
              )}
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
                  value={CONNECTION_STATUS_LABELS[connectionStatus] ?? "Unknown"}
                  status={connectionStatus}
                />
              </div>
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Content
                className="z-50 px-3 py-2 text-xs bg-surface-raised text-fg border border-border rounded-md shadow-lg"
                sideOffset={5}
              >
                <div className="space-y-1 font-mono">
                  <div>
                    <span className="text-fg-subtle">UI:&nbsp;</span> {
                      typeof __VIDEONODE_UI_VERSION__ !== 'undefined' ? __VIDEONODE_UI_VERSION__ : 'dev'
                    }
                  </div>
                </div>
                <Tooltip.Arrow className="fill-surface-raised" />
              </Tooltip.Content>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>


      </div>
    </div>
  );
}
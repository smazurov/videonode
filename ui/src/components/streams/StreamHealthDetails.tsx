import { StatusPill } from '../primitives/StatusPill';
import { poolStateToPill } from '../../lib/pool-status';
import { activityLabel, encoderActive, type ActivityHealth, type StreamHealth } from '../../lib/stream-health';

interface StreamHealthDetailsProps {
  readonly health: StreamHealth | undefined;
  readonly connected: boolean;
}

function ActivityRow({ label, activity }: {
  readonly label: string;
  readonly activity: ActivityHealth;
}) {
  const view = activityLabel(activity);
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
      <span className="text-fg-muted">{label}</span>
      <span className="flex items-center gap-2" title={view.detail}>
        <span className="font-mono text-fg-subtle">
          {activity.state !== 'unsupported' && activity.count}
          {activity.age_ms != null && ` · ${(activity.age_ms / 1000).toFixed(1)}s ago`}
        </span>
        <StatusPill status={view.tone} label={view.label} size="xs" showDot={false} />
      </span>
    </div>
  );
}

export function StreamHealthDetails({ health, connected }: StreamHealthDetailsProps) {
  if (!connected) {
    return <p className="text-sm text-fg-muted">Telemetry disconnected — media health is unknown.</p>;
  }
  if (!health) {
    return <p className="text-sm text-fg-muted">Waiting for encoder telemetry…</p>;
  }
  const active = encoderActive(health);
  const tracks = health.tracks ?? [];
  const audioTracks = tracks.filter((track) => track.type === 'audio');
  return (
    <div className="space-y-3 rounded-md border border-border p-3">
      <div className="flex items-center justify-between text-xs">
        <span className="uppercase tracking-wide text-fg-muted">FFmpeg publisher</span>
        <StatusPill
          status={poolStateToPill(health.encoder.state)}
          label={health.encoder.state}
          size="xs"
        />
      </div>
      {active ? (
        <>
          <ActivityRow label="Progress reports" activity={health.encoder.ffmpeg.progress} />
          <ActivityRow label="Video frames encoded" activity={health.encoder.ffmpeg.frames} />
          {health.publisher_started_at_us === 0 ? (
            <p className="text-xs text-fg-muted">Waiting for the publisher to announce media tracks.</p>
          ) : (
            <div className="space-y-3 border-t border-border pt-3">
              {tracks.map((track) => {
                const name = track.type === 'audio'
                  ? `Audio ${audioTracks.indexOf(track) + 1}`
                  : 'Video';
                return (
                  <div key={track.index} className="space-y-1">
                    <span className="text-xs font-medium text-fg">{name} · {track.codec}</span>
                    <ActivityRow label="Packets received" activity={track.packets} />
                    <ActivityRow label="Media delivered to fanout" activity={track.units} />
                  </div>
                );
              })}
              {audioTracks.length === 0 && <p className="text-xs text-fg-muted">No audio track advertised.</p>}
            </div>
          )}
        </>
      ) : (
        <p className="text-xs text-fg-muted">
          Encoder is {health.encoder.state}; media inactivity is not classified as a stall.
        </p>
      )}
      <p className="text-[10px] text-fg-subtle">
        Sampled {new Date(health.sampled_at).toLocaleTimeString()} · Stalled after {health.stall_after_ms / 1000}s without arrivals.
      </p>
    </div>
  );
}

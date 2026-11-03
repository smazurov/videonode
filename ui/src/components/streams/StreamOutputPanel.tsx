import { Link } from 'react-router-dom';
import toast from 'react-hot-toast';
import { ClipboardDocumentIcon } from '@heroicons/react/24/outline';
import { useStreamStore } from '../../hooks/useStreamStore';
import { Card } from '../Card';
import { Badge } from '../Badge';
import { SectionHeader } from '../primitives/SectionHeader';
import { KVInspector, type KVEntry } from '../primitives/KVInspector';
import { buildStreamURL } from '../../lib/api';
import { copyText } from '../../lib/clipboard';
import { RecordingControls } from './RecordingControls';

const ROW = 'group flex w-full items-center gap-3 rounded-md border border-border bg-surface-muted/30 px-3 py-2 text-left hover:border-fg-subtle';

function CopyRow({ tone, label, url }: {
  readonly tone: 'rtsp' | 'srt';
  readonly label: string;
  readonly url: string | undefined;
}) {
  return (
    <button
      type="button"
      disabled={!url}
      className={ROW}
      title={url ? 'Copy URL' : undefined}
      onClick={() => url && void copyText(url).then(() => toast.success(`${label} URL copied`))}
    >
      <Badge tone={tone} size="sm">{label}</Badge>
      <code className="min-w-0 flex-1 truncate font-mono text-xs text-fg" title={url}>{url ?? '—'}</code>
      {url && <ClipboardDocumentIcon className="h-4 w-4 shrink-0 text-fg-subtle group-hover:text-fg" />}
    </button>
  );
}

interface StreamOutputPanelProps {
  readonly streamId: string;
}

/** Where viewers connect, what they receive, and recording. */
export function StreamOutputPanel({ streamId }: StreamOutputPanelProps) {
  const stream = useStreamStore((s) => s.streamsById[streamId]);
  const encoder = stream?.encoder ?? {};
  const audio = stream?.audio;
  const devices = audio?.devices ?? [];
  const rtsp = stream?.rtsp_url ? (buildStreamURL(stream.rtsp_url, 'rtsp') ?? stream.rtsp_url) : undefined;
  const srt = stream?.srt_url ? (buildStreamURL(stream.srt_url, 'srt') ?? stream.srt_url) : undefined;
  // GOP / rate control / preset are nearly always defaults, so they live in
  // the tooltip rather than as rows.
  const tuning = [
    `GOP: ${encoder.gop ?? 'default'}`,
    `Rate control: ${encoder.rate_control ?? 'default'}`,
    `Preset: ${encoder.preset ?? 'default'}`,
  ].join('\n');
  const video = [encoder.codec?.toLowerCase(), encoder.bitrate].filter(Boolean).join(' ') || 'default';
  const sound = devices.length > 0 ? [audio?.codec, audio?.bitrate].filter(Boolean).join(' ') || 'default' : 'none';
  const entries: KVEntry[] = [
    { label: 'Video', value: <span title={tuning}>{video}</span> },
    { label: 'Audio', value: sound, ...(devices.length > 0 ? { hint: devices.join(', ') } : {}) },
  ];
  return (
    <Card padding="lg">
      <SectionHeader title="Output" />
      <div className="space-y-2">
        <CopyRow tone="rtsp" label="RTSP" url={rtsp} />
        <CopyRow tone="srt" label="SRT" url={srt} />
        <Link to={`/video?stream=${encodeURIComponent(streamId)}`} className={ROW}>
          <Badge tone="webrtc" size="sm">WebRTC</Badge>
          <span className="flex-1 text-xs text-fg-muted group-hover:text-fg">Open player →</span>
        </Link>
      </div>
      <div className="mt-5 border-t border-border pt-4">
        <KVInspector entries={entries} dense />
      </div>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-border pt-4">
        <span className="text-sm font-medium text-fg-muted">Recording</span>
        <RecordingControls streamId={streamId} bare />
      </div>
    </Card>
  );
}

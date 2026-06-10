import { Card } from '../Card';
import { SectionHeader } from '../primitives/SectionHeader';
import { KVInspector, type KVEntry } from '../primitives/KVInspector';
import type { SourceEntry } from '../../hooks/useSourceStore';

interface SourceFormatPanelProps {
  source: SourceEntry;
}

export function SourceFormatPanel({ source }: Readonly<SourceFormatPanelProps>) {
  const format = source.latest_status?.format;
  const device = source.latest_status?.device;
  const isPipe = !!source.pipe;

  const entries: KVEntry[] = format
    ? [
        { label: 'pixelformat', value: format.fourcc || '—' },
        { label: 'resolution', value: `${format.w}×${format.h}` },
        { label: 'framerate', value: `${format.fps} fps` },
        { label: 'mode', value: format.mode || '—' },
        { label: 'buffers', value: String(format.buffers) },
      ]
    : [];

  if (isPipe) {
    entries.push({ label: 'pipe_command', value: source.pipe, mono: true });
  } else if (device) {
    entries.push({ label: 'device_path', value: device.path });
    entries.push({ label: 'multiplanar', value: String(device.multiplanar) });
  }

  return (
    <Card padding="lg">
      <SectionHeader
        title="Format"
        description={isPipe ? 'Detected from the y4m pipe stream.' : 'Active V4L2 capture format.'}
      />
      <KVInspector entries={entries} emptyText="No format data yet" />
    </Card>
  );
}

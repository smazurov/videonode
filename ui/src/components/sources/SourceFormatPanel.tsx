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

  const entries: KVEntry[] = format
    ? [
        { key: 'pixelformat', value: format.fourcc || '—' },
        { key: 'resolution', value: `${format.w}×${format.h}` },
        { key: 'framerate', value: `${format.fps} fps` },
        { key: 'mode', value: format.mode || '—' },
        { key: 'buffers', value: String(format.buffers) },
      ]
    : [];

  if (device) {
    entries.push({ key: 'device_path', value: device.path });
    entries.push({ key: 'multiplanar', value: String(device.multiplanar) });
  }

  return (
    <Card padding="lg">
      <SectionHeader title="Format" description="Active V4L2 capture format." />
      <KVInspector entries={entries} emptyText="No format data yet" />
    </Card>
  );
}

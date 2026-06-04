import { Badge } from '../Badge';
import type { SensorFinding } from '../../hooks/useSensorStore';

interface SensorFindingsFeedProps {
  findings: SensorFinding[];
}

function decisionTone(decision: string): 'success' | 'warning' | 'neutral' {
  if (decision.startsWith('crop')) return 'success';
  if (decision.startsWith('widen')) return 'warning';
  return 'neutral';
}

// SensorFindingsFeed renders the live stream of findings the daemon pushes over
// SSE (`sensor.status`). This is the answer to "how do I know it's working?" —
// every detection shows up here with its confidence, the policy decision, and
// whether a crop was applied, updating frame by frame.
export function SensorFindingsFeed({ findings }: Readonly<SensorFindingsFeedProps>) {
  if (findings.length === 0) {
    return (
      <p className="text-sm text-fg-subtle">
        No findings yet. Once the sensor process is running and the pipeline switch is on,
        detections stream in here live.
      </p>
    );
  }

  return (
    <div className="overflow-hidden rounded-md border border-border">
      <table className="w-full text-xs">
        <thead className="bg-surface-muted text-fg-subtle">
          <tr>
            <th className="px-3 py-1.5 text-left font-medium">Frame</th>
            <th className="px-3 py-1.5 text-left font-medium">Kind</th>
            <th className="px-3 py-1.5 text-right font-medium">Conf.</th>
            <th className="px-3 py-1.5 text-left font-medium">Decision</th>
            <th className="px-3 py-1.5 text-left font-medium">Crop (x,y,scale)</th>
            <th className="px-3 py-1.5 text-center font-medium">Applied</th>
          </tr>
        </thead>
        <tbody>
          {findings.map((f) => (
            <tr
              key={`${f.frame_idx}-${f.target_ref}-${f.decision}-${f.confidence}`}
              className="border-t border-border"
            >
              <td className="px-3 py-1.5 font-mono text-fg-muted">{f.frame_idx}</td>
              <td className="px-3 py-1.5 text-fg-muted">{f.kind}</td>
              <td className="px-3 py-1.5 text-right font-mono text-fg-muted">
                {(f.confidence * 100).toFixed(0)}%
              </td>
              <td className="px-3 py-1.5">
                <Badge tone={decisionTone(f.decision)} size="xs">
                  {f.decision}
                </Badge>
              </td>
              <td className="px-3 py-1.5 font-mono text-fg-muted">
                {f.crop
                  ? `${f.crop.X?.toFixed(2)}, ${f.crop.Y?.toFixed(2)}, ${f.crop.Scale?.toFixed(2)}`
                  : '—'}
              </td>
              <td className="px-3 py-1.5 text-center">
                {f.applied ? (
                  <span className="text-success">✓</span>
                ) : (
                  <span className="text-fg-subtle">·</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

import { Link } from 'react-router-dom';
import { Badge } from '../Badge';
import { Card } from '../Card';
import { SectionHeader } from '../primitives/SectionHeader';
import { EmptyState } from '../primitives/EmptyState';
import type { SourceConsumerRef } from '../../hooks/useSourceStore';

interface SourceConsumersPanelProps {
  consumers: SourceConsumerRef[];
}

function consumerTo(consumer: SourceConsumerRef): string {
  if (consumer.kind === 'composer') return `/composers/${consumer.id}`;
  return `/streams/${consumer.id}/edit`;
}

export function SourceConsumersPanel({ consumers }: Readonly<SourceConsumersPanelProps>) {
  return (
    <Card padding="lg">
      <SectionHeader
        title="Consumers"
        description="Composers and streams that reference this source."
      />
      {consumers.length === 0 ? (
        <EmptyState
          title="No consumers"
          description="This source isn't currently referenced by any composer or stream."
        />
      ) : (
        <ul className="divide-y divide-border">
          {consumers.map((c) => (
            <li key={`${c.kind}:${c.id}`} className="py-2">
              <Link
                to={consumerTo(c)}
                className="flex items-center justify-between hover:bg-surface-muted/40 rounded px-2 -mx-2 py-1"
              >
                <div className="flex items-center gap-2">
                  <Badge tone={c.kind === 'composer' ? 'canvas' : 'webrtc'} size="xs">
                    {c.kind}
                  </Badge>
                  <span className="text-sm font-medium text-fg">{c.id}</span>
                </div>
                <span className="text-xs text-fg-muted">View →</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

import { Card } from './Card';
import { Badge } from './Badge';
import { CanvasMemberRow } from './CanvasMemberStreamCard';

interface CanvasMembersGroupCardProps {
  canvasId: string;
  streamIds: string[];
  className?: string;
}

export function CanvasMembersGroupCard({
  canvasId,
  streamIds,
  className = '',
}: Readonly<CanvasMembersGroupCardProps>) {
  return (
    <Card className={`h-full ${className}`}>
      <Card.Content className="space-y-3 py-3">
        <div className="flex items-center justify-between gap-2">
          <Badge tone="rtmp" size="xs" title={`Devices captured by canvas ${canvasId}`}>
            In canvas: {canvasId}
          </Badge>
          <span className="text-xs font-mono text-fg-muted">
            {streamIds.length} streams
          </span>
        </div>
        <div className="divide-y divide-border">
          {streamIds.map((streamId) => (
            <div key={streamId} className="py-3 first:pt-0 last:pb-0">
              <CanvasMemberRow streamId={streamId} />
            </div>
          ))}
        </div>
      </Card.Content>
    </Card>
  );
}

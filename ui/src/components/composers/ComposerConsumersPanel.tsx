import { Link } from 'react-router-dom';
import { Card } from '../Card';
import type { ComposerData } from '../../lib/composer-types';

interface ComposerConsumersPanelProps {
  composer: ComposerData;
}

export function ComposerConsumersPanel({ composer }: Readonly<ComposerConsumersPanelProps>) {
  const streamIds = composer.downstream_stream_ids ?? [];
  return (
    <Card padding="none">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold text-fg">Downstream streams</h2>
        <span className="text-xs text-fg-muted">{streamIds.length} total</span>
      </div>
      {streamIds.length === 0 ? (
        <div className="p-6 text-center text-sm text-fg-muted">
          No streams reference this composer.
        </div>
      ) : (
        <ul className="divide-y divide-border">
          {streamIds.map((id) => (
            <li key={id} className="px-4 py-3">
              <Link
                to={`/streams/${encodeURIComponent(id)}`}
                className="font-mono text-sm text-accent hover:underline"
              >
                {id}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

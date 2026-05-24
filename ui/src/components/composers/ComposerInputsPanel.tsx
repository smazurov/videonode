import { useEffect } from 'react';
import { Link } from 'react-router-dom';
import { Badge } from '../Badge';
import { Card } from '../Card';
import { StatusPill, type StatusPillStatus } from '../primitives/StatusPill';
import type { ComposerData, ComposerInput } from '../../lib/composer-types';
import { useSourceStore } from '../../hooks/useSourceStore';

interface ComposerInputsPanelProps {
  composer: ComposerData;
}

const SOURCE_REF_PREFIX = 'source:';

function sourceIdFromRef(ref: string): string | null {
  if (!ref.startsWith(SOURCE_REF_PREFIX)) return null;
  return ref.slice(SOURCE_REF_PREFIX.length);
}

function resolveStatus(sourceId: string | null, source: ReturnType<typeof useSourceStore.getState>['sourcesById'][string] | undefined): StatusPillStatus | 'missing' {
  if (!sourceId || !source) return 'missing';
  return source.status ?? 'idle';
}

function InputRow({ input }: Readonly<{ input: ComposerInput }>) {
  const sourceId = sourceIdFromRef(input.ref);
  const source = useSourceStore((s) => (sourceId ? s.sourcesById[sourceId] : undefined));
  // sourcesById is keyed by id; the resolved status (runtime field on
  // Source) drives the pill. When the source row is absent the composer
  // ref is dangling — render an explicit "missing" hint, not a vague
  // "unknown".
  const status = resolveStatus(sourceId, source);
  return (
    <tr className="border-t border-border">
      <td className="px-4 py-3 align-middle">
        {sourceId ? (
          <Link to={`/sources/${encodeURIComponent(sourceId)}`} className="font-mono text-sm text-accent hover:underline break-all">
            {input.ref}
          </Link>
        ) : (
          <span className="font-mono text-sm break-all">{input.ref}</span>
        )}
      </td>
      <td className="px-4 py-3 align-middle">
        {input.effect ? (
          <Badge tone="info" size="xs">{input.effect.type}</Badge>
        ) : (
          <span className="text-xs text-fg-subtle">none</span>
        )}
      </td>
      <td className="px-4 py-3 align-middle">
        {status === 'missing' ? (
          <Badge tone="danger" size="xs">missing</Badge>
        ) : (
          <StatusPill status={status} size="xs" />
        )}
      </td>
    </tr>
  );
}

export function ComposerInputsPanel({ composer }: Readonly<ComposerInputsPanelProps>) {
  // Pull sources into the store on first mount so the per-row status
  // resolves to something real instead of "missing".
  const fetchSources = useSourceStore((s) => s.fetchSources);
  const sourcesLastUpdated = useSourceStore((s) => s.lastUpdated);
  useEffect(() => {
    if (sourcesLastUpdated === null) void fetchSources();
  }, [sourcesLastUpdated, fetchSources]);
  return (
    <Card padding="none">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold text-fg">Inputs</h2>
        <span className="text-xs text-fg-muted">{composer.inputs.length} total</span>
      </div>
      {composer.inputs.length === 0 ? (
        <div className="p-6 text-center text-sm text-fg-muted">
          No inputs configured.
        </div>
      ) : (
        <table className="w-full border-collapse text-sm">
          <thead className="bg-surface-sunken">
            <tr>
              <th scope="col" className="px-4 py-2 text-left text-xs font-medium uppercase tracking-wide text-fg-muted">
                Ref
              </th>
              <th scope="col" className="px-4 py-2 text-left text-xs font-medium uppercase tracking-wide text-fg-muted">
                Effect
              </th>
              <th scope="col" className="px-4 py-2 text-left text-xs font-medium uppercase tracking-wide text-fg-muted">
                Source status
              </th>
            </tr>
          </thead>
          <tbody>
            {composer.inputs.map((input, idx) => (
              <InputRow key={`${input.ref}-${idx}`} input={input} />
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}

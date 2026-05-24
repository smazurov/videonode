import { Link } from 'react-router-dom';
import { Badge } from '../Badge';
import { Card } from '../Card';
import type { ComposerData, ComposerInput } from '../../lib/composer-types';

interface ComposerInputsPanelProps {
  composer: ComposerData;
}

const SOURCE_REF_PREFIX = 'source:';

function sourceIdFromRef(ref: string): string | null {
  if (!ref.startsWith(SOURCE_REF_PREFIX)) return null;
  return ref.slice(SOURCE_REF_PREFIX.length);
}

function InputRow({ input }: Readonly<{ input: ComposerInput }>) {
  const sourceId = sourceIdFromRef(input.ref);
  return (
    <tr className="border-t border-border">
      <td className="px-4 py-3 align-middle">
        {sourceId ? (
          <Link to={`/sources/${encodeURIComponent(sourceId)}`} className="font-mono text-sm text-accent hover:underline">
            {input.ref}
          </Link>
        ) : (
          <span className="font-mono text-sm">{input.ref}</span>
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
        {/* Source-status pill is best-effort — U6 wires the sources store
            that owns the real status. For now we render an "unknown" pill
            so the column is present and styled. */}
        <Badge tone="neutral" size="xs">unknown</Badge>
      </td>
    </tr>
  );
}

export function ComposerInputsPanel({ composer }: Readonly<ComposerInputsPanelProps>) {
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

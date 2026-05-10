import { semanticTokens, type SemanticTokenName } from "../tokens";

interface TokenGridProps {
  /** Filter by token name prefix, e.g. "surface", "fg", "accent". Omit for all. */
  readonly group?: string;
  readonly columns?: 2 | 3 | 4;
}

const COL_CLASS: Record<2 | 3 | 4, string> = {
  2: "md:grid-cols-2",
  3: "md:grid-cols-2 lg:grid-cols-3",
  4: "md:grid-cols-2 lg:grid-cols-4",
};

// Renders semantic tokens as swatches. Reads from the generated `semanticTokens`
// map so this can never drift from tokens.dtcg.json.
export function TokenGrid({ group, columns = 3 }: TokenGridProps) {
  const entries = (Object.entries(semanticTokens) as [
    SemanticTokenName,
    (typeof semanticTokens)[SemanticTokenName],
  ][]).filter(([name]) => (group ? name.startsWith(group) : true));

  const colClass = COL_CLASS[columns];

  return (
    <div className={`grid grid-cols-1 ${colClass} gap-2`}>
      {entries.map(([name, entry]) => (
        <div
          key={name}
          className="flex items-center gap-3 rounded-md border border-border bg-surface-raised p-3"
        >
          <div
            className="h-10 w-10 rounded border border-border-strong shrink-0"
            style={{ backgroundColor: entry.cssVar }}
            aria-hidden="true"
          />
          <div className="min-w-0 flex-1">
            <div className="font-mono text-sm text-fg truncate">{name}</div>
            <div className="font-mono text-[10px] text-fg-subtle truncate">
              L {entry.light} · D {entry.dark}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

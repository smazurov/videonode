import { ReactNode } from "react";
import { cn } from "../../utils";

export interface KVEntry {
  key: string;
  value: ReactNode;
}

interface KVInspectorProps {
  entries: KVEntry[];
  className?: string;
  emptyText?: string;
}

export function KVInspector({ entries, className, emptyText = "No data" }: Readonly<KVInspectorProps>) {
  if (entries.length === 0) {
    return <p className={cn("text-sm text-fg-muted", className)}>{emptyText}</p>;
  }
  return (
    <dl className={cn("grid grid-cols-[max-content_1fr] gap-x-4 gap-y-2 text-sm", className)}>
      {entries.map((entry) => (
        <div key={entry.key} className="contents">
          <dt className="text-fg-muted">{entry.key}</dt>
          <dd className="text-fg break-all font-mono text-xs">{entry.value ?? "—"}</dd>
        </div>
      ))}
    </dl>
  );
}

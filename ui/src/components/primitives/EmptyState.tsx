import { ReactNode } from "react";
import { cn } from "../../utils";

interface EmptyStateProps {
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ title, description, action, className }: Readonly<EmptyStateProps>) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center text-center py-12 px-6 border border-dashed border-border rounded-lg bg-surface-muted/40",
        className,
      )}
    >
      <p className="text-base font-medium text-fg">{title}</p>
      {description && <p className="mt-1 text-sm text-fg-muted max-w-md">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

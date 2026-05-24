import React from "react";
import { cn } from "../../utils";

type IconComponent = React.ComponentType<React.SVGProps<SVGSVGElement>>;

export interface EmptyStateProps {
  readonly icon?: IconComponent;
  readonly headline?: React.ReactNode;
  readonly title?: React.ReactNode; // alias for headline (back-compat)
  readonly description?: React.ReactNode;
  readonly cta?: React.ReactNode;
  readonly className?: string;
}

export function EmptyState({
  icon: Icon,
  headline,
  title,
  description,
  cta,
  className,
}: Readonly<EmptyStateProps>) {
  const effectiveHeadline = headline ?? title;
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center text-center py-12 px-6",
        "bg-surface border border-dashed border-border rounded-lg",
        className,
      )}
    >
      {Icon && (
        <div className="mb-4 inline-flex h-12 w-12 items-center justify-center rounded-full bg-surface-muted">
          <Icon className="h-6 w-6 text-fg-muted" />
        </div>
      )}
      <h3 className="text-base font-semibold text-fg">{effectiveHeadline}</h3>
      {description && (
        <p className="mt-2 max-w-sm text-sm text-fg-muted">{description}</p>
      )}
      {cta && <div className="mt-6">{cta}</div>}
    </div>
  );
}

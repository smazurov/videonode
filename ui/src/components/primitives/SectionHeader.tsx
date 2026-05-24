import React from "react";
import { cn } from "../../utils";

export interface SectionHeaderProps {
  readonly title: React.ReactNode;
  readonly description?: React.ReactNode;
  readonly actions?: React.ReactNode;
  readonly level?: 2 | 3 | 4;
  readonly className?: string;
}

export function SectionHeader({
  title,
  description,
  actions,
  level = 2,
  className,
}: Readonly<SectionHeaderProps>) {
  const headingClass = (() => {
    if (level === 2) return "text-lg font-semibold text-fg";
    if (level === 3) return "text-base font-semibold text-fg";
    return "text-sm font-semibold text-fg";
  })();

  const heading = (() => {
    if (level === 2) return <h2 className={headingClass}>{title}</h2>;
    if (level === 3) return <h3 className={headingClass}>{title}</h3>;
    return <h4 className={headingClass}>{title}</h4>;
  })();

  const content = (
    <div className="min-w-0">
      {heading}
      {description && <p className="mt-1 text-sm text-fg-muted">{description}</p>}
    </div>
  );

  return (
    <div className={cn("flex items-start justify-between gap-4", className)}>
      {content}
      {actions && <div className="shrink-0">{actions}</div>}
    </div>
  );
}

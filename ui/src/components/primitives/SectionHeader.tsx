<<<<<<< HEAD
import React from "react";
import { cn } from "../../utils";

export interface SectionHeaderProps {
  readonly title: React.ReactNode;
  readonly description?: React.ReactNode;
  readonly action?: React.ReactNode;
  readonly level?: 2 | 3 | 4;
  readonly className?: string;
}

export function SectionHeader({
  title,
  description,
  action,
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
      {action && <div className="shrink-0">{action}</div>}
=======
import type { ReactNode } from 'react';
import { cn } from '../../utils';

interface SectionHeaderProps {
  readonly title: ReactNode;
  readonly description?: ReactNode;
  readonly actions?: ReactNode;
  readonly className?: string;
}

export function SectionHeader({ title, description, actions, className }: SectionHeaderProps) {
  return (
    <div className={cn('flex items-start justify-between gap-3', className)}>
      <div className="min-w-0">
        <h3 className="text-sm font-semibold text-fg">{title}</h3>
        {description && <p className="mt-0.5 text-xs text-fg-muted">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
>>>>>>> worktree-agent-a00537f30a6b3ed35
    </div>
  );
}

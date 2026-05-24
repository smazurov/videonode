import type { ReactNode } from 'react';
import { ChevronRightIcon } from '@heroicons/react/24/outline';
import { Link } from 'react-router-dom';
import { cn } from '../../utils';

export interface BreadcrumbEntry {
  readonly label: ReactNode;
  readonly to?: string;
}

interface EntityDetailLayoutProps {
  readonly breadcrumbs?: ReadonlyArray<BreadcrumbEntry>;
  readonly title: ReactNode;
  readonly subtitle?: ReactNode;
  readonly actions?: ReactNode;
  readonly sidebar?: ReactNode;
  readonly children: ReactNode;
  readonly className?: string;
}

// Master-detail layout stub for U4. Provides breadcrumb + title + optional
// left sibling rail + main content. U4 will replace with the canonical
// implementation; keeping the props minimal so consumers don't churn.
export function EntityDetailLayout({
  breadcrumbs,
  title,
  subtitle,
  actions,
  sidebar,
  children,
  className,
}: EntityDetailLayoutProps) {
  return (
    <div className={cn('space-y-4', className)}>
      {breadcrumbs && breadcrumbs.length > 0 && (
        <nav aria-label="Breadcrumb" className="flex flex-wrap items-center gap-1 text-xs text-fg-muted">
          {breadcrumbs.map((entry, idx) => (
            <span key={idx} className="inline-flex items-center gap-1">
              {entry.to ? (
                <Link to={entry.to} className="hover:text-fg">
                  {entry.label}
                </Link>
              ) : (
                <span className="text-fg">{entry.label}</span>
              )}
              {idx < breadcrumbs.length - 1 && <ChevronRightIcon className="h-3 w-3 text-fg-subtle" />}
            </span>
          ))}
        </nav>
      )}

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-bold text-fg">{title}</h1>
          {subtitle && <div className="mt-1 text-sm text-fg-muted">{subtitle}</div>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>

      <div className={cn('grid gap-4', sidebar ? 'grid-cols-1 lg:grid-cols-[260px_minmax(0,1fr)]' : 'grid-cols-1')}>
        {sidebar && <aside className="min-w-0">{sidebar}</aside>}
        <div className="min-w-0 space-y-4">{children}</div>
      </div>
    </div>
  );
}

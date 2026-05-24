import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { cn } from '../utils';

export interface EntityDetailCrumb {
  label: string;
  to?: string;
}

interface EntityDetailLayoutProps {
  breadcrumbs: EntityDetailCrumb[];
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  siblingRail?: ReactNode;
  children: ReactNode;
  className?: string;
}

// Master-detail two-column primitive used by all entity detail pages.
// U4 will own this file; we ship a minimal version here so U8 isn't blocked.
export function EntityDetailLayout({
  breadcrumbs,
  title,
  subtitle,
  actions,
  siblingRail,
  children,
  className,
}: Readonly<EntityDetailLayoutProps>) {
  return (
    <div className={cn('flex flex-col gap-4', className)}>
      <nav className="flex items-center gap-1 text-xs text-fg-subtle" aria-label="Breadcrumb">
        {breadcrumbs.map((crumb, idx) => {
          const isLast = idx === breadcrumbs.length - 1;
          return (
            <span key={`${crumb.label}-${idx}`} className="flex items-center gap-1">
              {idx > 0 && <span aria-hidden="true">/</span>}
              {crumb.to && !isLast ? (
                <Link to={crumb.to} className="hover:text-fg">
                  {crumb.label}
                </Link>
              ) : (
                <span className={isLast ? 'text-fg' : ''}>{crumb.label}</span>
              )}
            </span>
          );
        })}
      </nav>

      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold text-fg truncate">{title}</h1>
          {subtitle && <p className="mt-1 text-sm text-fg-muted">{subtitle}</p>}
        </div>
        {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
      </div>

      <div className="grid grid-cols-12 gap-6">
        {siblingRail && (
          <aside className="col-span-12 lg:col-span-3">
            <div className="lg:sticky lg:top-6">{siblingRail}</div>
          </aside>
        )}
        <div className={cn('col-span-12', siblingRail ? 'lg:col-span-9' : 'lg:col-span-12')}>
          <div className="space-y-6">{children}</div>
        </div>
      </div>
    </div>
  );
}

import { ReactNode } from "react";
import { Link } from "react-router-dom";
import { cn } from "../utils";

// Stub for U4 — final implementation may add breadcrumbs/search.
export interface EntityListItem {
  id: string;
  label: string;
  to: string;
  sublabel?: string;
  active?: boolean;
}

interface BreadcrumbItem {
  label: string;
  to?: string;
}

interface EntityDetailLayoutProps {
  title: ReactNode;
  breadcrumbs?: BreadcrumbItem[];
  siblings?: EntityListItem[];
  siblingsTitle?: string;
  siblingsEmpty?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}

export function EntityDetailLayout({
  title,
  breadcrumbs,
  siblings,
  siblingsTitle = "All",
  siblingsEmpty,
  actions,
  children,
  className,
}: Readonly<EntityDetailLayoutProps>) {
  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {breadcrumbs && breadcrumbs.length > 0 && (
        <nav aria-label="Breadcrumb" className="text-sm text-fg-muted">
          <ol className="flex items-center gap-1.5">
            {breadcrumbs.map((b, i) => (
              <li key={`${b.label}-${i}`} className="flex items-center gap-1.5">
                {b.to ? (
                  <Link to={b.to} className="hover:text-fg">
                    {b.label}
                  </Link>
                ) : (
                  <span>{b.label}</span>
                )}
                {i < breadcrumbs.length - 1 && <span aria-hidden>/</span>}
              </li>
            ))}
          </ol>
        </nav>
      )}

      <div className="flex items-start justify-between gap-4">
        <div className="text-xl font-semibold text-fg">{title}</div>
        {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
      </div>

      <div className="grid grid-cols-12 gap-6">
        <aside className="col-span-12 lg:col-span-3 xl:col-span-3">
          <div className="lg:sticky lg:top-6">
            <div className="rounded-md border border-border bg-surface-raised overflow-hidden">
              <div className="px-3 py-2 border-b border-border text-xs font-medium text-fg-muted uppercase tracking-wide">
                {siblingsTitle}
              </div>
              {siblings && siblings.length > 0 ? (
                <ul className="max-h-[60vh] overflow-y-auto">
                  {siblings.map((item) => (
                    <li key={item.id}>
                      <Link
                        to={item.to}
                        className={cn(
                          "block px-3 py-2 text-sm border-b border-border last:border-b-0 hover:bg-surface-muted/60",
                          item.active ? "bg-surface-muted/80 font-medium" : "",
                        )}
                      >
                        <div className="text-fg truncate">{item.label}</div>
                        {item.sublabel && (
                          <div className="text-xs text-fg-muted truncate">{item.sublabel}</div>
                        )}
                      </Link>
                    </li>
                  ))}
                </ul>
              ) : (
                <div className="px-3 py-4 text-sm text-fg-muted">{siblingsEmpty ?? "Empty"}</div>
              )}
            </div>
          </div>
        </aside>

        <section className="col-span-12 lg:col-span-9 xl:col-span-9 space-y-4">{children}</section>
      </div>
    </div>
  );
}

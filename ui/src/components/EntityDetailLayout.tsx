import { ReactNode } from "react";
import { Link } from "react-router-dom";
import { cn } from "../utils";

interface EntityDetailLayoutProps {
  siblingList: ReactNode;
  breadcrumbs: ReactNode;
  children: ReactNode;
  backTo?: string;
  backLabel?: string;
  className?: string;
}

export function EntityDetailLayout({
  siblingList,
  breadcrumbs,
  children,
  backTo,
  backLabel = "Back",
  className,
}: Readonly<EntityDetailLayoutProps>) {
  return (
    <div className={cn("flex flex-col gap-4", className)}>
      <div className="flex items-center gap-3 text-sm text-fg-muted">
        {backTo && (
          <Link
            to={backTo}
            className="inline-flex items-center gap-1 rounded px-2 py-1 text-fg-muted hover:bg-surface-raised hover:text-fg"
          >
            <span aria-hidden="true">&larr;</span>
            <span>{backLabel}</span>
          </Link>
        )}
        <div className="flex min-w-0 flex-1 items-center">{breadcrumbs}</div>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[18rem_minmax(0,1fr)]">
        <aside className="lg:sticky lg:top-6 lg:self-start">
          <div className="rounded border border-border bg-surface-raised">
            {siblingList}
          </div>
        </aside>
        <section className="min-w-0">{children}</section>
      </div>
    </div>
  );
}

export default EntityDetailLayout;

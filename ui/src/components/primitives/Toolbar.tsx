import React from "react";
import { cn } from "../../utils";

export interface ToolbarProps {
  readonly left?: React.ReactNode;
  readonly right?: React.ReactNode;
  readonly children?: React.ReactNode;
  readonly className?: string;
  readonly bordered?: boolean;
}

export function Toolbar({
  left,
  right,
  children,
  className,
  bordered = true,
}: Readonly<ToolbarProps>) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 px-3 py-2",
        bordered ? "border-b border-border bg-surface" : undefined,
        className,
      )}
    >
      {left && <div className="flex items-center gap-2 min-w-0">{left}</div>}
      {children && <div className="flex-1 flex items-center gap-2 min-w-0">{children}</div>}
      {!children && <div className="flex-1" />}
      {right && <div className="flex items-center gap-2 shrink-0">{right}</div>}
    </div>
  );
}

import React from "react";

type StateName = "default" | "hover" | "focus-visible" | "active" | "disabled" | "loading" | "error";

interface StateRowProps {
  readonly states: readonly StateName[];
  readonly render: (state: StateName) => React.ReactNode;
  readonly label?: string;
}

// Renders one primitive in every requested state side-by-side. Hover/active/focus
// are simulated by native state — the consumer is responsible for passing the
// right props (e.g. `disabled={state === 'disabled'}`). For hover/focus-visible
// there's no pure-CSS way to force a state on a different element without JS,
// so we rely on the user hovering/tabbing. The row is still a valuable visual
// checklist of which states the primitive covers.
export function StateRow({ states, render, label }: StateRowProps) {
  return (
    <div className="flex flex-col gap-2">
      {label && <div className="text-xs font-mono text-fg-muted">{label}</div>}
      <div className="flex flex-wrap items-center gap-4">
        {states.map((state) => (
          <div key={state} className="flex flex-col items-center gap-1">
            <div>{render(state)}</div>
            <span className="text-[10px] font-mono text-fg-subtle">{state}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

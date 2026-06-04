import React, { useMemo, useState } from "react";
import {
  Combobox,
  ComboboxButton,
  ComboboxInput,
  ComboboxOption,
  ComboboxOptions,
} from "@headlessui/react";
import { ChevronUpDownIcon, CheckIcon } from "@heroicons/react/24/outline";
import { cn } from "../../utils";
import { StatusPill, type StatusPillStatus } from "./StatusPill";

export type UpstreamKind = "source" | "composer";

export interface UpstreamOption {
  /** Stable id within the kind, e.g. "hdmi-slides". */
  readonly id: string;
  /** Pre-built ref, e.g. "source:hdmi-slides". When omitted, derived as `${kind}:${id}`. */
  readonly ref?: string;
  readonly kind: UpstreamKind;
  readonly label?: string;
  readonly status?: StatusPillStatus;
  readonly disabled?: boolean;
}

export interface UpstreamPickerProps {
  readonly options: readonly UpstreamOption[];
  readonly value: string | null;
  readonly onChange: (next: string | null) => void;
  readonly placeholder?: string;
  readonly label?: string;
  readonly error?: string;
  readonly hint?: string;
  readonly disabled?: boolean;
  readonly className?: string;
  readonly id?: string;
}

const refFor = (opt: UpstreamOption): string => opt.ref ?? `${opt.kind}:${opt.id}`;

const KIND_ORDER: readonly UpstreamKind[] = ["source", "composer"];
const KIND_LABELS: Record<UpstreamKind, string> = {
  source: "Sources",
  composer: "Composers",
};

export function UpstreamPicker({
  options,
  value,
  onChange,
  placeholder = "Pick an upstream…",
  label,
  error,
  hint,
  disabled,
  className,
  id,
}: Readonly<UpstreamPickerProps>) {
  const [query, setQuery] = useState("");
  const reactId = React.useId();
  const inputId = id ?? reactId;
  const errorId = error ? `${inputId}-error` : undefined;
  const hintId = hint ? `${inputId}-hint` : undefined;

  const indexByRef = useMemo(() => {
    const m = new Map<string, UpstreamOption>();
    for (const o of options) m.set(refFor(o), o);
    return m;
  }, [options]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const matches = q
      ? options.filter((o) => {
          const ref = refFor(o);
          return (
            ref.toLowerCase().includes(q) ||
            (o.label ?? o.id).toLowerCase().includes(q)
          );
        })
      : options;
    const groups = new Map<UpstreamKind, UpstreamOption[]>();
    for (const o of matches) {
      const arr = groups.get(o.kind) ?? [];
      arr.push(o);
      groups.set(o.kind, arr);
    }
    return KIND_ORDER.flatMap((k) => {
      const items = groups.get(k);
      if (!items || items.length === 0) return [];
      return [{ header: k }, ...items.map((opt) => ({ opt }))];
    }) as ReadonlyArray<{ readonly header: UpstreamKind } | { readonly opt: UpstreamOption }>;
  }, [options, query]);

  const selected = value ? indexByRef.get(value) ?? null : null;

  const display = (ref: string | null): string => {
    if (!ref) return "";
    const opt = indexByRef.get(ref);
    if (!opt) return ref;
    return opt.label ? `${refFor(opt)} — ${opt.label}` : refFor(opt);
  };

  return (
    <div className={cn("space-y-1", className)}>
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-fg">
          {label}
        </label>
      )}
      <Combobox
        value={value}
        onChange={(next: string | null) => onChange(next)}
        disabled={disabled ?? false}
      >
        <div className="relative">
          <ComboboxInput
            id={inputId}
            aria-invalid={error ? true : undefined}
            aria-describedby={[errorId, hintId].filter(Boolean).join(" ") || undefined}
            displayValue={(ref: string | null) => display(ref)}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={placeholder}
            className={cn(
              "block w-full rounded-sm border border-border-strong px-3 py-2 pr-9 text-sm",
              "bg-surface text-fg placeholder:text-fg-subtle",
              "focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent",
              "disabled:opacity-50 disabled:cursor-not-allowed",
              error ? "border-danger focus-visible:ring-danger focus-visible:border-danger" : undefined,
            )}
          />
          <ComboboxButton className="absolute inset-y-0 right-0 flex items-center pr-2">
            <ChevronUpDownIcon className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
          </ComboboxButton>
          <ComboboxOptions
            className={cn(
              "absolute z-20 mt-1 max-h-72 w-full overflow-auto rounded-md bg-surface shadow-lg",
              "border border-border focus:outline-none text-sm",
            )}
          >
            {filtered.length === 0 ? (
              <div className="px-3 py-2 text-fg-muted text-xs">No matches</div>
            ) : (
              filtered.map((entry) => {
                if ("header" in entry) {
                  return (
                    <div
                      key={`header-${entry.header}`}
                      className="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-wide text-fg-subtle"
                    >
                      {KIND_LABELS[entry.header]}
                    </div>
                  );
                }
                const opt = entry.opt;
                const ref = refFor(opt);
                return (
                  <ComboboxOption
                    key={ref}
                    value={ref}
                    disabled={opt.disabled ?? false}
                    className={({ focus }) =>
                      cn(
                        "flex items-center justify-between gap-2 px-3 py-2 cursor-default",
                        focus ? "bg-accent-soft text-accent-soft-fg" : undefined,
                        opt.disabled ? "opacity-50 cursor-not-allowed" : undefined,
                      )
                    }
                  >
                    {({ selected: isSelected }) => (
                      <>
                        <span className="flex items-center gap-2 min-w-0">
                          {isSelected ? (
                            <CheckIcon className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                          ) : (
                            <span className="h-4 w-4 shrink-0" aria-hidden="true" />
                          )}
                          <span className="truncate font-mono text-xs">{ref}</span>
                          {opt.label && (
                            <span className="truncate text-fg-muted text-xs">— {opt.label}</span>
                          )}
                        </span>
                        {opt.status && <StatusPill status={opt.status} size="xs" />}
                      </>
                    )}
                  </ComboboxOption>
                );
              })
            )}
          </ComboboxOptions>
        </div>
      </Combobox>
      {selected?.status && (
        <p className="text-xs text-fg-muted">
          Status: <StatusPill status={selected.status} size="xs" />
        </p>
      )}
      {hint && !error && (
        <p id={hintId} className="text-xs text-fg-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} className="text-sm text-danger-soft-fg">
          {error}
        </p>
      )}
    </div>
  );
}

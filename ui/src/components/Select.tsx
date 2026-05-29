import React, { useId } from "react";
import { cn } from "../utils";

interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  error?: string;
  fullWidth?: boolean;
  hint?: string;
  ref?: React.Ref<HTMLSelectElement>;
}

export function Select({ label, error, fullWidth = true, hint, className, id, children, required, ref, ...props }: Readonly<SelectProps>) {
  const reactId = useId();
  const selectId = id ?? reactId;
  const errorId = error ? `${selectId}-error` : undefined;
  const hintId = hint ? `${selectId}-hint` : undefined;
  const describedBy = [errorId, hintId].filter(Boolean).join(" ") || undefined;

  return (
    <div className={cn("space-y-1", fullWidth ? "w-full" : "")}>
      {label && (
        <label htmlFor={selectId} className="block text-sm font-medium text-fg">
          {label}
          {required && <span className="ml-1 text-danger">*</span>}
        </label>
      )}
      <select
        ref={ref}
        id={selectId}
        required={required}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        className={cn(
          "block w-full rounded-md border border-border px-3 py-2 text-sm",
          "bg-surface text-fg",
          "focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          error && "border-danger focus-visible:ring-danger focus-visible:border-danger",
          className,
        )}
        {...props}
      >
        {children}
      </select>
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

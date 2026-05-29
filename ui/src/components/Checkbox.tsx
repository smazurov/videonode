import React, { useId, type Ref } from "react";
import { cn } from "../utils";

interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "type" | "size"> {
  readonly label?: React.ReactNode;
  readonly description?: React.ReactNode;
  readonly error?: string;
  readonly ref?: Ref<HTMLInputElement>;
}

export function Checkbox({ label, description, error, id, className, ref, ...props }: Readonly<CheckboxProps>) {
  const reactId = useId();
  const inputId = id ?? reactId;
  const descId = description ? `${inputId}-desc` : undefined;
  const errorId = error ? `${inputId}-error` : undefined;
  const describedBy = [descId, errorId].filter(Boolean).join(" ") || undefined;

  return (
    <span className={cn("inline-flex items-start gap-2", className)}>
      <input
        ref={ref}
        id={inputId}
        type="checkbox"
        aria-describedby={describedBy}
        aria-invalid={error ? true : undefined}
        className={cn(
          "mt-0.5 h-4 w-4 shrink-0 rounded border border-border-strong accent-accent",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring",
          "disabled:opacity-50 disabled:cursor-not-allowed",
        )}
        {...props}
      />
      {(label || description || error) && (
        <span className="flex flex-col min-w-0 text-sm leading-tight">
          {label && (
            <label htmlFor={inputId} className="text-fg select-none cursor-pointer">
              {label}
            </label>
          )}
          {description && (
            <span id={descId} className="text-xs text-fg-subtle mt-0.5">
              {description}
            </span>
          )}
          {error && (
            <span id={errorId} className="text-xs text-danger-soft-fg mt-0.5">
              {error}
            </span>
          )}
        </span>
      )}
    </span>
  );
}

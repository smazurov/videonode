import React, { useId } from "react";
import { cn } from "../utils";

interface InputFieldProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  fullWidth?: boolean;
  hint?: string;
  ref?: React.Ref<HTMLInputElement>;
}

export function InputField({ label, error, fullWidth, hint, className, id, required, ref, ...props }: Readonly<InputFieldProps>) {
  const reactId = useId();
  const inputId = id ?? reactId;
  const errorId = error ? `${inputId}-error` : undefined;
  const hintId = hint ? `${inputId}-hint` : undefined;
  const describedBy = [errorId, hintId].filter(Boolean).join(" ") || undefined;

  return (
    <div className={cn("space-y-1", fullWidth ? "w-full" : "")}>
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-fg">
          {label}
          {required && <span className="ml-1 text-danger">*</span>}
        </label>
      )}
      <input
        ref={ref}
        id={inputId}
        required={required}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        className={cn(
          "block w-full rounded-sm border border-border-strong px-3 py-2 text-sm",
          "bg-surface text-fg placeholder:text-fg-subtle",
          "focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          error && "border-danger focus-visible:ring-danger focus-visible:border-danger",
          className,
        )}
        {...props}
      />
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

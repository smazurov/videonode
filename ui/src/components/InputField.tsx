import React from "react";
import { cn } from "../utils";

interface InputFieldProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  fullWidth?: boolean;
}

export const InputField = React.forwardRef<HTMLInputElement, InputFieldProps>(
  ({ label, error, fullWidth, className, ...props }, ref) => {
    return (
      <div className={cn("space-y-1", fullWidth ? "w-full" : "")}>
        {label && (
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {label}
          </label>
        )}
        <input
          ref={ref}
          className={cn(
            "block w-full rounded-sm border border-slate-800/30 px-3 py-2 text-sm",
            "focus:border-blue-600 focus:outline-none focus:ring-1 focus:ring-blue-600",
            "dark:border-slate-300/20 dark:bg-slate-800 dark:text-white",
            "dark:focus:border-blue-500 dark:focus:ring-blue-500",
            error && "border-red-500 focus:border-red-500 focus:ring-red-500",
            className
          )}
          {...props}
        />
        {error && (
          <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
        )}
      </div>
    );
  }
);

InputField.displayName = "InputField";
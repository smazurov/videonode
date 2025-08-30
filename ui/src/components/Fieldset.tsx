import React from "react";
import { cn } from "../utils";

interface FieldsetProps {
  children: React.ReactNode;
  className?: string;
}

export default function Fieldset({ children, className }: Readonly<FieldsetProps>) {
  return (
    <fieldset className={cn("border rounded-sm p-6 space-y-4 bg-white dark:bg-gray-900", className)}>
      {children}
    </fieldset>
  );
}
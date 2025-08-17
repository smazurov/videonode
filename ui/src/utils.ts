import { twMerge } from "tailwind-merge";
import { cva, type VariantProps } from "cva";

/**
 * Utility for merging class names with tailwind-merge
 */
export function cn(...inputs: (string | undefined)[]) {
  return twMerge(inputs.filter(Boolean).join(" "));
}

/**
 * Class variance authority helper
 */
export { cva, type VariantProps };
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

/**
 * Truncate device ID to a reasonable length for display
 */
export function truncateDeviceId(deviceId: string, maxLength: number = 30): string {
  if (deviceId.length <= maxLength) {
    return deviceId;
  }
  return deviceId.slice(0, maxLength) + '...';
}

/**
 * Icon size tokens — single source of truth for heroicon h-N w-N pairs.
 * Used by Button's iconVariants and by hand-coded icon call sites.
 */
export const ICON_SIZE = {
  SM: "h-4 w-4",
  MD: "h-5 w-5",
  LG: "h-6 w-6",
} as const;

export type IconSize = keyof typeof ICON_SIZE;
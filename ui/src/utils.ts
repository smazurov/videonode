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
import type { Story, StoryDefault } from "@ladle/react";
import { StatusPill, type StatusPillStatus } from "./StatusPill";
import { VariantMatrix } from "../../design/blocks/VariantMatrix";

export default {
  title: "Primitives/StatusPill",
} satisfies StoryDefault;

const STATUSES: readonly StatusPillStatus[] = [
  "warm",
  "cold",
  "error",
  "encoding",
  "idle",
  "running",
  "stopped",
];
const SIZES = ["xs", "sm", "md"] as const;

export const StatusBySize: Story = () => (
  <VariantMatrix
    rowLabel="status"
    colLabel="size"
    rows={STATUSES}
    cols={SIZES}
    render={(status, size) => <StatusPill status={status} size={size} />}
  />
);

export const WithoutDot: Story = () => (
  <div className="flex flex-wrap gap-2">
    {STATUSES.map((status) => (
      <StatusPill key={status} status={status} showDot={false} />
    ))}
  </div>
);

export const CustomLabels: Story = () => (
  <div className="flex flex-wrap gap-2">
    <StatusPill status="warm" label="Producer warm (3 readers)" />
    <StatusPill status="encoding" label="H.265 @ 12 Mbps" />
    <StatusPill status="error" label="VAAPI init failed" />
  </div>
);

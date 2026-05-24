import type { Story, StoryDefault } from "@ladle/react";
import { KVInspector } from "./KVInspector";
import { StatusPill } from "./StatusPill";

export default {
  title: "Primitives/KVInspector",
} satisfies StoryDefault;

const ITEMS = [
  { label: "ID", value: <code className="font-mono text-xs">hdmi-slides</code> },
  { label: "Device", value: "/dev/video0", hint: "rk3588-hdmi-rx" },
  { label: "Format", value: "NV12 1920×1080@60" },
  { label: "Status", value: <StatusPill status="warm" /> },
  { label: "Consumers", value: 3 },
];

export const Regular: Story = () => (
  <div className="max-w-md">
    <KVInspector items={ITEMS} />
  </div>
);

export const Dense: Story = () => (
  <div className="max-w-md">
    <KVInspector items={ITEMS} dense />
  </div>
);

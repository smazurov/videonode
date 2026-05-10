import type { Story, StoryDefault } from "@ladle/react";
import { Spinner } from "./Spinner";

export default {
  title: "Feedback/Spinner",
} satisfies StoryDefault;

export const Sizes: Story = () => (
  <div className="flex flex-wrap items-center gap-6">
    {(["xs", "sm", "md", "lg"] as const).map((size) => (
      <div key={size} className="flex flex-col items-center gap-2">
        <Spinner size={size} />
        <span className="text-xs text-fg-muted font-mono">{size}</span>
      </div>
    ))}
  </div>
);

export const Tones: Story = () => (
  <div className="flex flex-wrap items-center gap-6">
    <div className="flex flex-col items-center gap-2">
      <Spinner tone="accent" />
      <span className="text-xs text-fg-muted font-mono">accent</span>
    </div>
    <div className="flex flex-col items-center gap-2">
      <Spinner tone="muted" />
      <span className="text-xs text-fg-muted font-mono">muted</span>
    </div>
    <div className="flex flex-col items-center gap-2 text-danger">
      <Spinner tone="current" />
      <span className="text-xs text-fg-muted font-mono">current (danger)</span>
    </div>
  </div>
);

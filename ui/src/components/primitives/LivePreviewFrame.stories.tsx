import type { Story, StoryDefault } from "@ladle/react";
import { LivePreviewFrame } from "./LivePreviewFrame";

export default {
  title: "Primitives/LivePreviewFrame",
} satisfies StoryDefault;

const placeholder = (
  <div className="w-full h-full bg-accent-soft flex items-center justify-center text-accent-soft-fg text-sm font-mono">
    &lt;video /&gt;
  </div>
);

export const Ready: Story = () => (
  <div className="max-w-2xl">
    <LivePreviewFrame state="ready">{placeholder}</LivePreviewFrame>
  </div>
);

export const Loading: Story = () => (
  <div className="max-w-2xl">
    <LivePreviewFrame state="loading">{placeholder}</LivePreviewFrame>
  </div>
);

export const ErrorState: Story = () => (
  <div className="max-w-2xl">
    <LivePreviewFrame state="error" errorMessage="WebRTC peer dropped" />
  </div>
);

export const Idle: Story = () => (
  <div className="max-w-2xl">
    <LivePreviewFrame state="idle" idleMessage="Stream not started" />
  </div>
);

export const WithStats: Story = () => (
  <div className="max-w-2xl">
    <LivePreviewFrame
      state="ready"
      stats={<span>60fps · 12.3 Mbps</span>}
      statsPosition="bottom-right"
    >
      {placeholder}
    </LivePreviewFrame>
  </div>
);

export const PortraitAspect: Story = () => (
  <div className="max-w-xs">
    <LivePreviewFrame aspectRatio={9 / 16} state="ready">
      {placeholder}
    </LivePreviewFrame>
  </div>
);

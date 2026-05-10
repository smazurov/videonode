import type { Story, StoryDefault } from "@ladle/react";
import { TokenGrid } from "./blocks/TokenGrid";

export default {
  title: "Design System/Tokens",
} satisfies StoryDefault;

function TokenSection({ title, group }: { readonly title: string; readonly group: string }) {
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold text-fg">{title}</h3>
      <TokenGrid group={group} />
    </section>
  );
}

export const Surfaces: Story = () => <TokenGrid group="surface" />;

export const Foreground: Story = () => <TokenGrid group="fg" />;

export const Borders: Story = () => (
  <div className="space-y-4">
    <TokenGrid group="border" />
    <TokenGrid group="focus" />
  </div>
);

export const Accent: Story = () => <TokenGrid group="accent" />;

export const Status: Story = () => (
  <div className="space-y-4">
    <TokenSection title="danger" group="danger" />
    <TokenSection title="warning" group="warning" />
    <TokenSection title="success" group="success" />
    <TokenSection title="info" group="info" />
  </div>
);

export const FeatureAccents: Story = () => (
  <div className="space-y-4">
    <TokenSection title="canvas" group="canvas" />
    <TokenSection title="webrtc" group="webrtc" />
    <TokenSection title="rtsp" group="rtsp" />
    <TokenSection title="srt" group="srt" />
    <TokenSection title="rtmp" group="rtmp" />
  </div>
);

export const LogLevels: Story = () => <TokenGrid group="log" />;

export const All: Story = () => (
  <div className="space-y-2">
    <p className="text-sm text-fg-muted">
      All values from <code className="font-mono">src/design/tokens.dtcg.json</code>. Swatch background
      is the live CSS variable; L / D show the resolved light/dark values. Toggle the Ladle theme (top
      toolbar) to see swatches swap in place.
    </p>
    <TokenGrid />
  </div>
);

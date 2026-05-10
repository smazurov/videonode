import type { Story, StoryDefault } from "@ladle/react";
import { Badge } from "./Badge";
import { VariantMatrix } from "../design/blocks/VariantMatrix";

export default {
  title: "Feedback/Badge",
} satisfies StoryDefault;

const TONES = [
  "neutral",
  "info",
  "success",
  "warning",
  "danger",
  "accent",
  "canvas",
  "webrtc",
  "rtsp",
  "srt",
  "rtmp",
] as const;
const SIZES = ["xs", "sm", "md"] as const;

export const ToneBySize: Story = () => (
  <VariantMatrix
    rowLabel="tone"
    colLabel="size"
    rows={TONES}
    cols={SIZES}
    render={(tone, size) => (
      <Badge tone={tone} size={size}>
        {tone}
      </Badge>
    )}
  />
);

export const AllTones: Story = () => (
  <div className="flex flex-wrap gap-2">
    {TONES.map((tone) => (
      <Badge key={tone} tone={tone} size="md">
        {tone}
      </Badge>
    ))}
  </div>
);

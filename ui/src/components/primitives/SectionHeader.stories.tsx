import type { Story, StoryDefault } from "@ladle/react";
import { SectionHeader } from "./SectionHeader";
import { Button } from "../Button";

export default {
  title: "Primitives/SectionHeader",
} satisfies StoryDefault;

export const Basic: Story = () => (
  <div className="space-y-6 max-w-2xl">
    <SectionHeader title="Sources" />
    <SectionHeader title="Composers" description="GLES composition of N sources onto a single canvas." />
    <SectionHeader
      title="Streams"
      description="Encoders with their publish targets."
      actions={<Button text="New stream" theme="primary" size="SM" />}
    />
    <SectionHeader title="Settings" level={3} description="Tier 3 heading." />
    <SectionHeader title="Advanced" level={4} />
  </div>
);

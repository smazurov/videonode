import { useState } from "react";
import type { Story, StoryDefault } from "@ladle/react";
import { UpstreamPicker, type UpstreamOption } from "./UpstreamPicker";

export default {
  title: "Primitives/UpstreamPicker",
} satisfies StoryDefault;

const OPTIONS: readonly UpstreamOption[] = [
  { kind: "source", id: "hdmi-slides", label: "HDMI capture", status: "warm" },
  { kind: "source", id: "cam-host", label: "Host webcam", status: "cold" },
  { kind: "source", id: "test-pattern", label: "Test pattern", status: "running" },
  { kind: "composer", id: "main-scene", label: "Main scene", status: "warm" },
  { kind: "composer", id: "wide-shot", label: "Wide shot", status: "cold" },
  { kind: "composer", id: "broken", label: "Broken composer", status: "error", disabled: true },
];

export const Basic: Story = () => {
  const [value, setValue] = useState<string | null>("composer:main-scene");
  return (
    <div className="max-w-md">
      <UpstreamPicker
        label="Upstream"
        options={OPTIONS}
        value={value}
        onChange={setValue}
        hint="Pick a source or composer to feed this stream."
      />
    </div>
  );
};

export const Empty: Story = () => {
  const [value, setValue] = useState<string | null>(null);
  return (
    <div className="max-w-md">
      <UpstreamPicker options={OPTIONS} value={value} onChange={setValue} />
    </div>
  );
};

export const ErrorState: Story = () => {
  const [value, setValue] = useState<string | null>(null);
  return (
    <div className="max-w-md">
      <UpstreamPicker
        label="Upstream"
        options={OPTIONS}
        value={value}
        onChange={setValue}
        error="Upstream is required."
      />
    </div>
  );
};

export const Disabled: Story = () => (
  <div className="max-w-md">
    <UpstreamPicker
      label="Upstream"
      options={OPTIONS}
      value="source:hdmi-slides"
      onChange={() => {}}
      disabled
    />
  </div>
);

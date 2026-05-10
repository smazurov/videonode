import type { Story, StoryDefault } from "@ladle/react";
import { useState } from "react";
import { Checkbox } from "./Checkbox";

export default {
  title: "Forms/Checkbox",
} satisfies StoryDefault;

export const States: Story = () => {
  const [a, setA] = useState(true);
  const [b, setB] = useState(false);
  return (
    <div className="space-y-3 max-w-md">
      <Checkbox checked={a} onChange={(e) => setA(e.target.checked)} label="Plain label" />
      <Checkbox
        checked={b}
        onChange={(e) => setB(e.target.checked)}
        label="With description"
        description="Subtext explains what this option does."
      />
      <Checkbox checked disabled label="Disabled (checked)" />
      <Checkbox disabled label="Disabled (unchecked)" />
      <Checkbox defaultChecked={false} label="With error" error="This setting conflicts with X" />
    </div>
  );
};

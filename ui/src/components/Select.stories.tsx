import type { Story, StoryDefault } from "@ladle/react";
import { Select } from "./Select";

export default {
  title: "Forms/Select",
} satisfies StoryDefault;

export const Default: Story = () => (
  <div className="max-w-sm">
    <Select label="Default" hint="Choose one">
      <option>Option A</option>
      <option>Option B</option>
      <option>Option C</option>
    </Select>
  </div>
);

export const Required: Story = () => (
  <div className="max-w-sm">
    <Select label="Required" required>
      <option value="">Select...</option>
      <option>Option A</option>
    </Select>
  </div>
);

export const WithError: Story = () => (
  <div className="max-w-sm">
    <Select label="With error" error="Please pick a value">
      <option>Option A</option>
    </Select>
  </div>
);

export const Disabled: Story = () => (
  <div className="max-w-sm">
    <Select label="Disabled" disabled>
      <option>Locked</option>
    </Select>
  </div>
);

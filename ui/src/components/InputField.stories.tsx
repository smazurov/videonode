import type { Story, StoryDefault } from "@ladle/react";
import { InputField } from "./InputField";
import { StateRow } from "../design/blocks/StateRow";

export default {
  title: "Forms/InputField",
} satisfies StoryDefault;

export const States: Story = () => (
  <div className="max-w-md">
    <StateRow
      states={["default", "disabled", "error"]}
      render={(state) => {
        if (state === "error") {
          return (
            <InputField
              label="With error"
              defaultValue="bad"
              error="Must be at least 4 characters"
              fullWidth
            />
          );
        }
        if (state === "disabled") {
          return <InputField label="Disabled" defaultValue="locked" disabled fullWidth />;
        }
        return <InputField label="Default" placeholder="type here..." fullWidth />;
      }}
    />
  </div>
);

export const WithHint: Story = () => (
  <div className="max-w-sm">
    <InputField
      label="Required field"
      required
      placeholder="name"
      hint="Shown below the input"
      fullWidth
    />
  </div>
);

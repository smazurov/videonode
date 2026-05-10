import type { Story, StoryDefault } from "@ladle/react";
import { PlusIcon } from "@heroicons/react/24/outline";
import { Button } from "./Button";
import { VariantMatrix } from "../design/blocks/VariantMatrix";
import { StateRow } from "../design/blocks/StateRow";

export default {
  title: "Actions/Button",
} satisfies StoryDefault;

const THEMES = ["primary", "danger", "light", "blank"] as const;
const SIZES = ["SM", "MD", "LG"] as const;

export const ThemeBySize: Story = () => (
  <VariantMatrix
    rowLabel="theme"
    colLabel="size"
    rows={THEMES}
    cols={SIZES}
    render={(theme, size) => (
      <Button theme={theme} size={size} text={`${theme} ${size}`} />
    )}
  />
);

export const WithIcon: Story = () => (
  <div className="flex flex-wrap items-center gap-2">
    {THEMES.map((theme) => (
      <Button key={theme} theme={theme} size="MD" text="Create" LeadingIcon={PlusIcon} />
    ))}
  </div>
);

export const States: Story = () => (
  <div className="space-y-4">
    {THEMES.map((theme) => (
      <StateRow
        key={theme}
        label={theme}
        states={["default", "disabled", "loading"]}
        render={(state) => (
          <Button
            theme={theme}
            size="MD"
            text={state}
            disabled={state === "disabled"}
            loading={state === "loading"}
          />
        )}
      />
    ))}
  </div>
);

export const FullWidth: Story = () => (
  <div className="max-w-sm space-y-2">
    <Button theme="primary" size="MD" text="Full width primary" fullWidth />
    <Button theme="light" size="MD" text="Full width light" fullWidth />
  </div>
);

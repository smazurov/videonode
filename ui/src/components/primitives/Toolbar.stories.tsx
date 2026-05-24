import type { Story, StoryDefault } from "@ladle/react";
import { Toolbar } from "./Toolbar";
import { Button } from "../Button";
import { InputField } from "../InputField";

export default {
  title: "Primitives/Toolbar",
} satisfies StoryDefault;

export const LeftAndRight: Story = () => (
  <Toolbar
    left={<h2 className="text-sm font-semibold text-fg">Sources (3)</h2>}
    right={<Button text="New" theme="primary" size="SM" />}
  />
);

export const WithSearch: Story = () => (
  <Toolbar
    left={<h2 className="text-sm font-semibold text-fg">Streams</h2>}
    right={
      <>
        <Button text="Refresh" theme="light" size="SM" />
        <Button text="New stream" theme="primary" size="SM" />
      </>
    }
  >
    <InputField placeholder="Filter…" fullWidth={false} className="w-64" />
  </Toolbar>
);

export const Unbordered: Story = () => (
  <Toolbar
    bordered={false}
    left={<span className="text-xs text-fg-muted">5 selected</span>}
    right={<Button text="Delete" theme="danger" size="SM" />}
  />
);

import type { Story, StoryDefault } from "@ladle/react";
import { Card } from "./Card";

export default {
  title: "Layout/Card",
} satisfies StoryDefault;

export const Default: Story = () => (
  <Card className="max-w-md">
    <Card.Content>Default card body.</Card.Content>
  </Card>
);

export const WithHeaderAndFooter: Story = () => (
  <Card className="max-w-md">
    <Card.Header>
      <h3 className="text-sm font-semibold text-fg">Card.Header</h3>
    </Card.Header>
    <Card.Content>Default body text in Card.Content.</Card.Content>
    <Card.Footer>Footer slot</Card.Footer>
  </Card>
);

export const Paddings: Story = () => (
  <div className="space-y-3 max-w-md">
    {(["none", "sm", "md", "lg"] as const).map((padding) => (
      <Card key={padding} padding={padding}>
        <div className="font-mono text-xs text-fg-muted">padding={padding}</div>
      </Card>
    ))}
  </div>
);

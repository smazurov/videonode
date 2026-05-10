import type { Story, StoryDefault } from "@ladle/react";
import { useState } from "react";
import { BottomSheet } from "./BottomSheet";
import { Button } from "./Button";
import { Badge } from "./Badge";

export default {
  title: "Overlay/BottomSheet",
} satisfies StoryDefault;

export const Basic: Story = () => {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button theme="primary" size="MD" text="Open sheet" onClick={() => setOpen(true)} />
      <BottomSheet
        open={open}
        onClose={() => setOpen(false)}
        title="Example sheet"
        maxWidth="2xl"
      >
        <p className="text-sm text-fg-muted mb-4">
          This is a BottomSheet. Content slots in via children.
        </p>
        <div className="flex justify-end gap-2">
          <Button theme="light" size="SM" text="Cancel" onClick={() => setOpen(false)} />
          <Button theme="primary" size="SM" text="OK" onClick={() => setOpen(false)} />
        </div>
      </BottomSheet>
    </>
  );
};

export const WithHeaderExtra: Story = () => {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        theme="primary"
        size="MD"
        text="Open with header badge"
        onClick={() => setOpen(true)}
      />
      <BottomSheet
        open={open}
        onClose={() => setOpen(false)}
        title="Custom command"
        maxWidth="4xl"
        headerExtra={<Badge tone="warning">Custom</Badge>}
      >
        <p className="text-sm text-fg-muted">
          The <code className="font-mono">headerExtra</code> slot sits next to the title.
        </p>
      </BottomSheet>
    </>
  );
};

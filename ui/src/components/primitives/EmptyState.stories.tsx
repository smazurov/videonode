import type { Story, StoryDefault } from "@ladle/react";
import { VideoCameraIcon, FilmIcon } from "@heroicons/react/24/outline";
import { EmptyState } from "./EmptyState";
import { Button } from "../Button";

export default {
  title: "Primitives/EmptyState",
} satisfies StoryDefault;

export const NoSources: Story = () => (
  <div className="max-w-xl">
    <EmptyState
      icon={VideoCameraIcon}
      title="No sources yet"
      description="Sources produce raw NV12 frames from V4L2 devices. Add one to start encoding."
      cta={<Button text="Add source" theme="primary" size="MD" />}
    />
  </div>
);

export const Plain: Story = () => (
  <div className="max-w-xl">
    <EmptyState title="No matches" />
  </div>
);

export const WithIconNoCta: Story = () => (
  <div className="max-w-xl">
    <EmptyState icon={FilmIcon} title="No recordings" description="Recordings will appear here once at least one stream has been captured." />
  </div>
);

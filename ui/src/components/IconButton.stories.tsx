import type { Story, StoryDefault } from "@ladle/react";
import {
  PencilSquareIcon,
  ArrowPathIcon,
  TrashIcon,
  XMarkIcon,
} from "@heroicons/react/24/outline";
import { IconButton } from "./IconButton";
import { VariantMatrix } from "../design/blocks/VariantMatrix";

export default {
  title: "Actions/IconButton",
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
      <IconButton icon={PencilSquareIcon} label="Edit" theme={theme} size={size} />
    )}
  />
);

export const Icons: Story = () => (
  <div className="flex flex-wrap items-center gap-2">
    <IconButton icon={PencilSquareIcon} label="Edit" />
    <IconButton icon={ArrowPathIcon} label="Restart" />
    <IconButton icon={TrashIcon} label="Delete" theme="danger" />
    <IconButton icon={XMarkIcon} label="Close" />
  </div>
);

export const Disabled: Story = () => (
  <div className="flex flex-wrap items-center gap-2">
    {THEMES.map((theme) => (
      <IconButton key={theme} icon={XMarkIcon} label={`${theme} disabled`} theme={theme} disabled />
    ))}
  </div>
);

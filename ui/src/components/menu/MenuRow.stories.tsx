import type { Story, StoryDefault } from "@ladle/react";
import { Menu, MenuButton, MenuItems } from "@headlessui/react";
import {
  CodeBracketIcon,
  DocumentTextIcon,
  TrashIcon,
  ViewfinderCircleIcon,
} from "@heroicons/react/24/outline";
import { EllipsisVerticalIcon } from "@heroicons/react/24/solid";
import { MenuRow } from "./MenuRow";
import { MENU_DOTS_BUTTON_CLASS } from "./menuStyles";
import { ICON_SIZE } from "../../utils";

export default {
  title: "Menu/MenuRow",
} satisfies StoryDefault;

export const OpenMenu: Story = () => (
  <Menu as="div" className="relative inline-block">
    <MenuButton title="More actions" className={MENU_DOTS_BUTTON_CLASS}>
      <EllipsisVerticalIcon className={`${ICON_SIZE.SM} shrink-0`} />
    </MenuButton>
    <MenuItems
      anchor="bottom start"
      className="z-50 mt-1 min-w-[220px] rounded border border-border bg-surface-raised py-1 shadow-lg focus:outline-none"
    >
      <MenuRow icon={CodeBracketIcon} label="Edit Command" onClick={() => {}} />
      <MenuRow icon={DocumentTextIcon} label="View Logs" onClick={() => {}} />
      <MenuRow icon={ViewfinderCircleIcon} label="Perspective" onClick={() => {}} disabled />
      <MenuRow icon={TrashIcon} label="Delete" onClick={() => {}} variant="danger" />
    </MenuItems>
  </Menu>
);

import React from "react";
import { MenuItem } from "@headlessui/react";
import { cn, ICON_SIZE } from "../../utils";
import {
  MENU_ROW_BASE,
  MENU_ROW_NEUTRAL,
  MENU_ROW_FOCUS,
  MENU_ROW_DANGER,
  MENU_ROW_DANGER_FOCUS,
} from "./menuStyles";

type IconComponent = React.ComponentType<{ className?: string }>;

interface MenuRowProps {
  readonly icon?: IconComponent;
  readonly label: React.ReactNode;
  readonly onClick: () => void;
  readonly variant?: "neutral" | "danger";
  readonly disabled?: boolean;
  readonly title?: string;
  readonly trailing?: React.ReactNode;
}

// Wraps Headless UI's MenuItem render-prop with the tokenized row styling.
// Consumer writes: <MenuRow icon={X} label="Foo" onClick={fn} />
export function MenuRow({
  icon: Icon,
  label,
  onClick,
  variant = "neutral",
  disabled = false,
  title,
  trailing,
}: MenuRowProps) {
  const isDanger = variant === "danger";
  const tone = isDanger ? MENU_ROW_DANGER : MENU_ROW_NEUTRAL;
  const focusTone = isDanger ? MENU_ROW_DANGER_FOCUS : MENU_ROW_FOCUS;

  return (
    <MenuItem disabled={disabled}>
      {({ focus, disabled: itemDisabled }) => (
        <button
          type="button"
          onClick={onClick}
          disabled={itemDisabled}
          title={title}
          className={cn(
            MENU_ROW_BASE,
            tone,
            focus && !itemDisabled ? focusTone : "",
            itemDisabled ? "cursor-not-allowed opacity-50" : "",
          )}
        >
          {Icon && <Icon className={`${ICON_SIZE.SM} shrink-0`} />}
          <span className="flex-1">{label}</span>
          {trailing}
        </button>
      )}
    </MenuItem>
  );
}

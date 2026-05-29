import React from "react";
import { cva, cn, ICON_SIZE, type IconSize, type VariantProps } from "../utils";

const sizes = {
  SM: "h-8 w-8",
  MD: "h-10 w-10",
  LG: "h-12 w-12",
};

const themes = {
  primary:
    "bg-accent text-accent-fg border border-accent-hover hover:bg-accent-hover active:bg-accent-active",
  danger:
    "bg-danger text-danger-fg border border-danger-hover hover:bg-danger-hover active:bg-danger-hover",
  light:
    "bg-surface text-fg border border-border-strong hover:bg-surface-muted active:bg-surface-muted",
  blank:
    "bg-transparent text-fg border border-transparent hover:bg-surface-muted hover:border-border-strong",
};

const iconButtonVariants = cva({
  base: cn(
    "inline-flex items-center justify-center rounded-md shrink-0 select-none",
    "transition-colors duration-200",
    "outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
    "disabled:opacity-50 disabled:pointer-events-none",
  ),
  variants: { size: sizes, theme: themes },
  defaultVariants: { size: "MD", theme: "blank" },
});

type IconComponent = React.ComponentType<React.SVGProps<SVGSVGElement>>;

type Variants = VariantProps<typeof iconButtonVariants>;

interface IconButtonProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "children" | "aria-label"> {
  readonly icon: IconComponent;
  readonly label: string; // required for accessibility — rendered as aria-label + title
  readonly size?: Variants["size"];
  readonly theme?: Variants["theme"];
  readonly iconSize?: IconSize;
  readonly ref?: React.Ref<HTMLButtonElement>;
}

export function IconButton({
  icon: Icon,
  label,
  size = "MD",
  theme = "blank",
  iconSize = "MD",
  className,
  type = "button",
  ref,
  ...props
}: Readonly<IconButtonProps>) {
  return (
    <button
      ref={ref}
      type={type}
      aria-label={label}
      title={props.title ?? label}
      className={cn(iconButtonVariants({ size, theme }), className)}
      {...props}
    >
      <Icon className={cn(ICON_SIZE[iconSize], "shrink-0")} aria-hidden="true" />
    </button>
  );
}

import React, { JSX } from "react";
import { Link, LinkProps } from "react-router-dom";
import { cva, cn, ICON_SIZE } from "../utils";

const sizes = {
  SM: "h-[36px] px-3 text-[13px]",
  MD: "h-[40px] px-3.5 text-sm",
  LG: "h-[48px] px-4 text-base",
};

const themes = {
  primary: cn(
    "bg-accent text-accent-fg border border-accent-hover shadow-sm",
    "group-hover:bg-accent-hover",
    "group-active:bg-accent-active",
  ),
  danger: cn(
    "bg-danger text-danger-fg border border-danger-hover shadow-xs",
    "group-hover:bg-danger-hover group-hover:border-danger-hover",
    "group-active:bg-danger-hover",
  ),
  light: cn(
    "bg-surface text-fg border border-border-strong shadow-xs",
    "group-hover:bg-surface-muted",
    "group-active:bg-surface-muted",
    "group-disabled:group-hover:bg-surface",
  ),
  blank: cn(
    "bg-transparent text-fg border border-transparent",
    "group-hover:bg-surface group-hover:border-border-strong group-hover:shadow-sm",
    "group-active:bg-surface-muted",
  ),
};

const btnVariants = cva({
  base: cn(
    "border rounded-sm select-none",
    "justify-center items-center shrink-0",
    "outline-none transition-colors duration-200",
    "font-medium text-center leading-tight",
    "group-focus-visible:outline-none group-focus-visible:ring-2 group-focus-visible:ring-offset-2 group-focus-visible:ring-focus-ring group-focus-visible:ring-offset-surface",
    "group-disabled:opacity-50 group-disabled:pointer-events-none",
  ),
  variants: { size: sizes, theme: themes },
});

const iconVariants = cva({
  variants: {
    size: ICON_SIZE,
    theme: {
      primary: "text-accent-fg",
      danger: "text-danger-fg",
      light: "text-fg",
      blank: "text-fg",
    },
  },
});

type IconComponent = React.ComponentType<React.SVGProps<SVGSVGElement>>;

interface ButtonContentPropsType {
  text?: string | React.ReactNode;
  LeadingIcon?: IconComponent | null;
  TrailingIcon?: IconComponent | null;
  fullWidth?: boolean;
  className?: string;
  textAlign?: "left" | "center" | "right";
  size: keyof typeof sizes;
  theme: keyof typeof themes;
  loading?: boolean;
}

function ButtonContent(props: Readonly<ButtonContentPropsType>) {
  const { text, LeadingIcon, TrailingIcon, fullWidth, className, textAlign, loading } = props;
  const iconClassName = iconVariants(props);
  const btnClassName = btnVariants(props);

  return (
    <div className={cn(className, fullWidth ? "flex" : "inline-flex", btnClassName)}>
      <div
        className={cn(
          "flex w-full min-w-0 items-center gap-x-1.5 text-center",
          textAlign === "left" ? "text-left" : "",
          textAlign === "center" ? "text-center" : "",
          textAlign === "right" ? "text-right" : "",
        )}
      >
        {loading ? (
          <div>
            <div
              className={cn(
                iconClassName,
                "animate-spin w-4 h-4 border-2 border-current border-t-transparent rounded-full",
              )}
            />
          </div>
        ) : (
          LeadingIcon && (
            <LeadingIcon className={cn(iconClassName, "shrink-0 justify-start")} />
          )
        )}

        {text && typeof text === "string" ? (
          <span className="relative w-full truncate">{text}</span>
        ) : (
          text
        )}

        {TrailingIcon && (
          <TrailingIcon className={cn(iconClassName, "shrink-0 justify-end")} />
        )}
      </div>
    </div>
  );
}

type ButtonPropsType = Pick<
  JSX.IntrinsicElements["button"],
  | "type"
  | "disabled"
  | "onClick"
  | "name"
  | "value"
  | "formNoValidate"
  | "onMouseLeave"
  | "onMouseDown"
  | "onMouseUp"
  | "title"
> &
  React.ComponentProps<typeof ButtonContent>;

export function Button({
  ref,
  type,
  disabled,
  onClick,
  formNoValidate,
  loading,
  ...props
}: ButtonPropsType & { ref?: React.Ref<HTMLButtonElement> }) {
  const classes = cn(
    "group outline-none",
    props.fullWidth ? "w-full" : "",
    loading ? "pointer-events-none" : "",
  );

  return (
    <button
      ref={ref}
      formNoValidate={formNoValidate}
      className={classes}
      type={type}
      disabled={disabled}
      onClick={onClick}
      onMouseLeave={props?.onMouseLeave}
      onMouseDown={props?.onMouseDown}
      onMouseUp={props?.onMouseUp}
      name={props.name}
      value={props.value}
      title={props.title}
    >
      <ButtonContent {...props} loading={loading ?? false} />
    </button>
  );
}

type LinkPropsType = Pick<LinkProps, "to"> &
  React.ComponentProps<typeof ButtonContent> & { disabled?: boolean };

export const LinkButton = ({ to, ...props }: LinkPropsType) => {
  const classes = cn(
    "group outline-none",
    props.disabled ? "pointer-events-none opacity-70" : "",
    props.fullWidth ? "w-full" : "",
    props.loading ? "pointer-events-none" : "",
    props.className,
  );

  return (
    <Link to={to} className={classes}>
      <ButtonContent {...props} />
    </Link>
  );
};

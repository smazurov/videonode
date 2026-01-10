import React, { JSX } from "react";
import { Link, LinkProps } from "react-router-dom";
import { cva, cn } from "../utils";

const sizes = {
  XS: "h-[28px] px-2 text-xs",
  SM: "h-[36px] px-3 text-[13px]",
  MD: "h-[40px] px-3.5 text-sm",
  LG: "h-[48px] px-4 text-base",
  XL: "h-[56px] px-5 text-base",
};

const themes = {
  primary: cn(
    // Base styles
    "bg-blue-700 dark:border-blue-600 border border-blue-900/60 text-white shadow-sm",
    // Hover states
    "group-hover:bg-blue-800",
    // Active states
    "group-active:bg-blue-900",
  ),
  danger: cn(
    // Base styles
    "bg-red-600 text-white border-red-700 shadow-xs shadow-red-200/80 dark:border-red-600 dark:shadow-red-900/20",
    // Hover states
    "group-hover:bg-red-700 group-hover:border-red-800 dark:group-hover:bg-red-700 dark:group-hover:border-red-600",
    // Active states
    "group-active:bg-red-800 dark:group-active:bg-red-800",
    // Focus states
    "group-focus:ring-red-700 dark:group-focus:ring-red-600",
  ),
  light: cn(
    // Base styles
    "bg-white text-black border-slate-800/30 shadow-xs dark:bg-slate-800 dark:border-slate-300/20 dark:text-white",
    // Hover states
    "group-hover:bg-blue-50/80 dark:group-hover:bg-slate-700",
    // Active states
    "group-active:bg-blue-100/60 dark:group-active:bg-slate-600",
    // Disabled states
    "group-disabled:group-hover:bg-white dark:group-disabled:group-hover:bg-slate-800",
  ),
  blank: cn(
    // Base styles
    "bg-white/0 text-black border-transparent dark:text-white",
    // Hover states
    "group-hover:bg-white group-hover:border-slate-800/30 group-hover:shadow-sm dark:group-hover:bg-slate-700 dark:group-hover:border-slate-600",
    // Active states
    "group-active:bg-slate-100/80",
  ),
};

const btnVariants = cva({
  base: cn(
    // Base styles
    "border rounded-sm select-none",
    // Size classes
    "justify-center items-center shrink-0",
    // Transition classes
    "outline-none transition-colors duration-200",
    // Text classes
    "font-medium text-center leading-tight",
    // States
    "group-focus:outline-none group-focus:ring-2 group-focus:ring-offset-2 group-focus:ring-blue-700",
    "group-disabled:opacity-50 group-disabled:pointer-events-none",
  ),

  variants: {
    size: sizes,
    theme: themes,
  },
});

const iconVariants = cva({
  variants: {
    size: {
      XS: "h-3.5",
      SM: "h-3.5",
      MD: "h-5",
      LG: "h-6",
      XL: "h-6",
    },
    theme: {
      primary: "text-white",
      danger: "text-white ",
      light: "text-black dark:text-white",
      blank: "text-black dark:text-white",
    },
  },
});

interface ButtonContentPropsType {
  text?: string | React.ReactNode;
  LeadingIcon?: React.FC<{ className: string | undefined }> | null;
  TrailingIcon?: React.FC<{ className: string | undefined }> | null;
  fullWidth?: boolean;
  className?: string;
  textAlign?: "left" | "center" | "right";
  size: keyof typeof sizes;
  theme: keyof typeof themes;
  loading?: boolean;
}

function ButtonContent(props: Readonly<ButtonContentPropsType>) {
  const { text, LeadingIcon, TrailingIcon, fullWidth, className, textAlign, loading } =
    props;

  // Based on the size prop, we'll use the corresponding variant classnames
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
            <div className={cn(iconClassName, "animate-spin w-4 h-4 border-2 border-current border-t-transparent rounded-full")} />
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

export const Button = React.forwardRef<HTMLButtonElement, ButtonPropsType>(
  ({ type, disabled, onClick, formNoValidate, loading, ...props }, ref) => {
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
        <ButtonContent
          {...props}
          loading={loading ?? false}
        />
      </button>
    );
  },
);

Button.displayName = "Button";

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
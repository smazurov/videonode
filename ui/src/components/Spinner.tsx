import { cva, cn, type VariantProps } from "../utils";

const spinnerVariants = cva({
  base: "inline-block rounded-full border-current border-t-transparent animate-spin shrink-0",
  variants: {
    size: {
      xs: "h-3 w-3 border-2",
      sm: "h-4 w-4 border-2",
      md: "h-6 w-6 border-2",
      lg: "h-8 w-8 border-[3px]",
    },
    tone: {
      accent: "text-accent",
      current: "text-current",
      muted: "text-fg-muted",
    },
  },
  defaultVariants: { size: "md", tone: "accent" },
});

type Variants = VariantProps<typeof spinnerVariants>;

interface SpinnerProps {
  readonly size?: Variants["size"];
  readonly tone?: Variants["tone"];
  readonly label?: string;
  readonly className?: string;
}

export function Spinner({ size, tone, label = "Loading", className }: Readonly<SpinnerProps>) {
  return (
    <span role="status" aria-label={label} className={cn(spinnerVariants({ size, tone }), className)}>
      <span className="sr-only">{label}</span>
    </span>
  );
}

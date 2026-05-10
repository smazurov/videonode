import { ReactNode } from "react";
import { cn } from "../utils";

const paddingStyles = {
  none: "",
  sm: "p-3",
  md: "p-4",
  lg: "p-6",
} as const;

interface CardProps {
  children: ReactNode;
  className?: string;
  padding?: keyof typeof paddingStyles;
}

interface CardHeaderProps {
  children: ReactNode;
  className?: string;
}

interface CardContentProps {
  children: ReactNode;
  className?: string;
}

interface CardFooterProps {
  children: ReactNode;
  className?: string;
}

export function Card({ children, className, padding = "md" }: Readonly<CardProps>) {
  return (
    <div
      className={cn(
        "bg-surface-raised border border-border rounded-lg shadow-sm",
        paddingStyles[padding],
        className,
      )}
    >
      {children}
    </div>
  );
}

export function CardHeader({ children, className }: Readonly<CardHeaderProps>) {
  return (
    <div className={cn("border-b border-border pb-3 mb-4", className)}>{children}</div>
  );
}

export function CardContent({ children, className }: Readonly<CardContentProps>) {
  return <div className={cn(className)}>{children}</div>;
}

export function CardFooter({ children, className }: Readonly<CardFooterProps>) {
  return (
    <div className={cn("border-t border-border pt-3 mt-4", className)}>{children}</div>
  );
}

Card.Header = CardHeader;
Card.Content = CardContent;
Card.Footer = CardFooter;

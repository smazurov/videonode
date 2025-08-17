import { ReactNode } from "react";
import { cn } from "../utils";

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

const paddingStyles = {
  none: "",
  sm: "p-3",
  md: "p-4", 
  lg: "p-6",
} as const;

export function Card({ children, className, padding = "md" }: Readonly<CardProps>) {
  return (
    <div className={cn(
      "bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg shadow-sm",
      paddingStyles[padding],
      className
    )}>
      {children}
    </div>
  );
}

export function CardHeader({ children, className }: Readonly<CardHeaderProps>) {
  return (
    <div className={cn("border-b border-slate-200 dark:border-slate-700 pb-3 mb-4", className)}>
      {children}
    </div>
  );
}

export function CardContent({ children, className }: Readonly<CardContentProps>) {
  return (
    <div className={cn("", className)}>
      {children}
    </div>
  );
}

export function CardFooter({ children, className }: Readonly<CardFooterProps>) {
  return (
    <div className={cn("border-t border-slate-200 dark:border-slate-700 pt-3 mt-4", className)}>
      {children}
    </div>
  );
}

// Export as compound component
Card.Header = CardHeader;
Card.Content = CardContent;  
Card.Footer = CardFooter;
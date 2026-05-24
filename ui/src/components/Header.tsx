import { Link } from "react-router-dom";
import { Button } from "./Button";
import Container from "./Container";
import { PipelineToggle } from "./PipelineToggle";
import { cn } from "../utils";

interface HeaderProps {
  onLogout?: () => void;
  className?: string;
}

export function Header({ onLogout, className }: Readonly<HeaderProps>) {
  
  return (
    <header className={cn("bg-surface-raised border-b border-border shadow-sm", className)}>
      <Container>
        <div className="flex items-center justify-between h-16">
          {/* Logo and branding */}
          <div className="flex items-center space-x-8">
            <div className="flex items-center space-x-2">
              <PipelineToggle />
              <Link to="/" className="text-xl font-bold text-fg">
                VideoNode
              </Link>
            </div>

            {/* Navigation */}
            <nav className="flex items-center space-x-4">
              <Link
                to="/streams"
                className="text-sm font-medium text-fg-muted hover:text-fg"
              >
                Streams
              </Link>
              <Link
                to="/logs"
                className="text-sm font-medium text-fg-muted hover:text-fg"
              >
                Logs
              </Link>
            </nav>
          </div>

          {/* Actions */}
          <div className="flex items-center space-x-3">
            {/* Logout button */}
            {onLogout && (
              <Button
                text="Logout"
                theme="light"
                size="SM"
                onClick={onLogout}
              />
            )}
          </div>
        </div>
      </Container>
    </header>
  );
}
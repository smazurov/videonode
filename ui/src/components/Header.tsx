import { Link, NavLink } from "react-router-dom";
import { Button } from "./Button";
import Container from "./Container";
import { PipelineToggle } from "./PipelineToggle";
import { cn } from "../utils";

interface HeaderProps {
  onLogout?: () => void;
  className?: string;
}

const NAV_TABS = [
  { to: "/sources", label: "Sources" },
  { to: "/composers", label: "Composers" },
  { to: "/streams", label: "Streams" },
  { to: "/logs", label: "Logs" },
] as const;

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
              {NAV_TABS.map((tab) => (
                <NavLink
                  key={tab.to}
                  to={tab.to}
                  className={({ isActive }) =>
                    cn(
                      "text-sm font-medium transition-colors",
                      isActive ? "text-fg" : "text-fg-muted hover:text-fg",
                    )
                  }
                >
                  {tab.label}
                </NavLink>
              ))}
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
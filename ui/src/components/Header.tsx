import { Link, NavLink } from "react-router-dom";
import { Button } from "./Button";
import Container from "./Container";
import { PipelineToggle } from "./PipelineToggle";
import { useConnectionStatus } from "../hooks/useConnectionStatus";
import { cn } from "../utils";

interface HeaderProps {
  onLogout?: () => void;
  className?: string;
}

const NAV_TABS = [
  { to: "/sources", label: "Sources" },
  { to: "/composers", label: "Composers" },
  { to: "/streams", label: "Streams" },
  { to: "/recordings", label: "Recordings" },
  { to: "/logs", label: "Logs" },
] as const;

export function Header({ onLogout, className }: Readonly<HeaderProps>) {
  const status = useConnectionStatus();

  // Subtle full-header tint when the backend connection drops: yellow while a
  // connection is actively being attempted (reconnecting), red while sitting
  // disconnected.
  let bgClass = "bg-surface-raised";
  let statusText: string | null = null;
  let statusColor = "";
  if (status === "reconnecting") {
    bgClass = "bg-warning/20";
    statusText = "Reconnecting…";
    statusColor = "text-warning-soft-fg";
  } else if (status === "offline") {
    bgClass = "bg-danger/20";
    statusText = "Disconnected";
    statusColor = "text-danger";
  }

  return (
    <header className={cn("border-b border-border shadow-sm transition-colors duration-500", bgClass, className)}>
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
            {statusText && (
              <span className={cn("text-sm font-medium", statusColor)}>{statusText}</span>
            )}
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

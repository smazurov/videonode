import { Link } from "react-router-dom";
import { Button } from "./Button";
import Container from "./Container";
import { cn } from "../utils";

interface HeaderProps {
  onLogout?: () => void;
  className?: string;
}

export function Header({ onLogout, className }: Readonly<HeaderProps>) {
  
  return (
    <header className={cn("bg-white dark:bg-slate-800 border-b border-slate-200 dark:border-slate-700 shadow-sm", className)}>
      <Container>
        <div className="flex items-center justify-between h-16">
          {/* Logo and branding */}
          <div className="flex items-center space-x-8">
            <Link to="/" className="flex items-center space-x-2">
              <div className="w-8 h-8 bg-blue-600 rounded-sm flex items-center justify-center">
                <span className="text-white font-bold text-sm">VN</span>
              </div>
              <span className="text-xl font-bold text-gray-900 dark:text-white">
                VideoNode
              </span>
            </Link>

            {/* Navigation */}
            <nav className="flex items-center space-x-4">
              <Link
                to="/streams"
                className="text-sm font-medium text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white"
              >
                Streams
              </Link>
              <Link
                to="/logs"
                className="text-sm font-medium text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white"
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
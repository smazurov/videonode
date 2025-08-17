import { Link } from "react-router-dom";
import { Button } from "./Button";
import Container from "./Container";
import { cn } from "../utils";

interface HeaderProps {
  onLogout?: () => void;
  onToggleStats?: () => void;
  className?: string;
}

export function Header({ onLogout, onToggleStats, className }: Readonly<HeaderProps>) {
  
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
            
          </div>

          {/* Actions */}
          <div className="flex items-center space-x-3">
            {/* Stats button */}
            {onToggleStats && (
              <Button
                text="System Stats"
                theme="light"
                size="SM"
                onClick={onToggleStats}
                LeadingIcon={({ className }) => (
                  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                  </svg>
                )}
              />
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
import { ReactNode } from "react";
import { Header } from "./Header";
import Container from "./Container";
import { cn } from "../utils";

interface DashboardLayoutProps {
  children: ReactNode;
  sidebar?: ReactNode;
  bottomBar?: ReactNode;
  onLogout?: () => void;
  onToggleStats?: () => void;
  className?: string;
}

interface MainContentProps {
  children: ReactNode;
  className?: string;
}

interface SidebarProps {
  children: ReactNode;
  className?: string;
}

interface BottomBarProps {
  children: ReactNode;
  className?: string;
}

export function DashboardLayout({ 
  children, 
  sidebar, 
  bottomBar, 
  onLogout, 
  onToggleStats,
  className 
}: Readonly<DashboardLayoutProps>) {
  return (
    <div className={cn("h-screen flex flex-col bg-gray-50 dark:bg-gray-900", className)}>
      {/* Header - fixed */}
      <Header {...(onLogout && { onLogout })} {...(onToggleStats && { onToggleStats })} />
      
      {/* Main content area - scrollable */}
      <div className="flex-1 overflow-y-auto">
        <Container>
          <div className="grid grid-cols-12 gap-6 py-6">
            {/* Main content area */}
            <div className={cn(
              "col-span-12",
              sidebar ? "lg:col-span-8" : "lg:col-span-12"
            )}>
              {children}
            </div>
            
            {/* Sidebar */}
            {sidebar && (
              <div className="col-span-12 lg:col-span-4">
                <div className="lg:sticky lg:top-6">
                  {sidebar}
                </div>
              </div>
            )}
          </div>
        </Container>
      </div>
      
      {/* Bottom info bar - fixed */}
      {bottomBar && (
        <div className="border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 z-10">
          {bottomBar}
        </div>
      )}
    </div>
  );
}

function SpacedContainer({ children, className }: Readonly<{ children: ReactNode; className?: string }>) {
  return (
    <div className={cn("space-y-6", className)}>
      {children}
    </div>
  );
}

export function MainContent({ children, className }: Readonly<MainContentProps>) {
  return <SpacedContainer {...(className && { className })}>{children}</SpacedContainer>;
}

export function Sidebar({ children, className }: Readonly<SidebarProps>) {
  return <SpacedContainer {...(className && { className })}>{children}</SpacedContainer>;
}

export function BottomBar({ children, className }: Readonly<BottomBarProps>) {
  return (
    <div className={cn("flex items-center justify-between", className)}>
      {children}
    </div>
  );
}

// Export as compound component
DashboardLayout.MainContent = MainContent;
DashboardLayout.Sidebar = Sidebar;
DashboardLayout.BottomBar = BottomBar;
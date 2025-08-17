import { Navigate, useLocation } from "react-router-dom";
import { useAuthStore } from "../hooks/useAuthStore";

interface ProtectedRouteProps {
  children: React.ReactNode;
}

export default function ProtectedRoute({ children }: Readonly<ProtectedRouteProps>) {
  const { user } = useAuthStore();
  const location = useLocation();

  if (!user?.isAuthenticated) {
    // Redirect to login page with returnTo parameter
    return (
      <Navigate 
        to={`/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`} 
        replace 
      />
    );
  }

  return <>{children}</>;
}
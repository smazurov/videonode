import { ReactNode, Suspense } from "react";
import ProtectedRoute from "./ProtectedRoute";
import { Spinner } from "./Spinner";

export function RouteFallback() {
  return (
    <div className="flex h-full min-h-[12rem] w-full items-center justify-center p-6">
      <Spinner />
    </div>
  );
}

export function Guarded({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <ProtectedRoute>
      <Suspense fallback={<RouteFallback />}>{children}</Suspense>
    </ProtectedRoute>
  );
}

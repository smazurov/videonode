import { Outlet } from "react-router-dom";

import { useAppSSEConnection } from "./hooks/useSSEManager";
import { VersionUpdateBanner } from "./components/VersionUpdateBanner";

function Root() {
  useAppSSEConnection();

  return (
    <div className="h-full w-full">
      <main className="h-full">
        <Outlet />
      </main>
      <VersionUpdateBanner />
    </div>
  );
}

export default Root;
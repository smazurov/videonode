import { Outlet } from "react-router-dom";

import { useAppSSEConnection } from "./hooks/useSSEManager";

function Root() {
  useAppSSEConnection();

  return (
    <div className="h-full w-full">
      <main className="h-full">
        <Outlet />
      </main>
    </div>
  );
}

export default Root;
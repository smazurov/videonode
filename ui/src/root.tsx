import { Outlet } from "react-router-dom";

function Root() {
  return (
    <div className="h-full w-full">
      <main className="h-full">
        <Outlet />
      </main>
    </div>
  );
}

export default Root;
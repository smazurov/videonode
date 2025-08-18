import ReactDOM from "react-dom/client";
import "./index.css";
import { createBrowserRouter, RouterProvider, Navigate } from "react-router-dom";
import { Toaster } from "react-hot-toast";

import Root from "./root";
import LoginRoute from "./routes/login";
import Streams from "./routes/streams";
import CreateStream from "./routes/create-stream";
import ProtectedRoute from "./components/ProtectedRoute";
import ErrorBoundary from "./components/ErrorBoundary";

// Create router with authentication
const router = createBrowserRouter([
  {
    path: "/login",
    element: <LoginRoute />,
    errorElement: <ErrorBoundary />,
  },
  {
    path: "/",
    element: <Root />,
    errorElement: <ErrorBoundary />,
    children: [
      {
        index: true,
        element: <Navigate to="/streams" replace />,
      },
      {
        path: "streams",
        element: (
          <ProtectedRoute>
            <Streams />
          </ProtectedRoute>
        ),
      },
      {
        path: "streams/new",
        element: (
          <ProtectedRoute>
            <CreateStream />
          </ProtectedRoute>
        ),
      }
    ]
  },
]);

document.addEventListener("DOMContentLoaded", () => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <>
      <RouterProvider router={router} />
      <Toaster
        toastOptions={{
          className:
            "rounded-sm border-none bg-white text-black shadow-sm outline-1 outline-slate-800/30",
        }}
        position="top-right"
      />
    </>,
  );
});

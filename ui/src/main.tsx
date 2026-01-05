import ReactDOM from "react-dom/client";
import "./index.css";
import { createBrowserRouter, RouterProvider, Navigate } from "react-router-dom";
import { Toaster } from "react-hot-toast";

import Root from "./root";
import LoginRoute from "./routes/login";
import VideoRoute from "./routes/video";
import Streams from "./routes/streams";
import CreateStream from "./routes/create-stream";
import EditStream from "./routes/edit-stream";
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
    path: "/video",
    element: <VideoRoute />,
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
      },
      {
        path: "streams/:streamId/edit",
        element: (
          <ProtectedRoute>
            <EditStream />
          </ProtectedRoute>
        ),
      }
    ]
  },
]);

document.addEventListener("DOMContentLoaded", () => {
  const isDarkMode = document.documentElement.classList.contains('dark');
  
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <>
      <RouterProvider router={router} />
      <Toaster
        toastOptions={{
          style: {
            background: isDarkMode ? '#1f2937' : '#ffffff',
            color: isDarkMode ? '#f9fafb' : '#111827',
            border: 'none',
            borderRadius: '0.125rem',
            boxShadow: '0 1px 2px 0 rgb(0 0 0 / 0.05)',
          },
        }}
        position="top-center"
      />
    </>,
  );
});

import { lazy } from "react";
import ReactDOM from "react-dom/client";
import "./index.css";
import { createBrowserRouter, RouterProvider, Navigate } from "react-router-dom";
import { Toaster } from "react-hot-toast";

import Root from "./root";
import LoginRoute from "./routes/login";
import VideoRoute from "./routes/video";
import ErrorBoundary from "./components/ErrorBoundary";
import { Guarded } from "./components/RouteGuards";

const Sources = lazy(() => import("./routes/sources"));
const CreateSource = lazy(() => import("./routes/create-source"));
const SourceDetail = lazy(() => import("./routes/source-detail"));
const EditSource = lazy(() => import("./routes/edit-source"));

const Composers = lazy(() => import("./routes/composers"));
const CreateComposer = lazy(() => import("./routes/create-composer"));
const ComposerDetail = lazy(() => import("./routes/composer-detail"));

const Streams = lazy(() => import("./routes/streams"));
const CreateStream = lazy(() => import("./routes/create-stream"));
const StreamDetail = lazy(() => import("./routes/stream-detail"));
const EditStream = lazy(() => import("./routes/edit-stream"));

const Logs = lazy(() => import("./routes/logs"));

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

      // Sources
      { path: "sources", element: <Guarded><Sources /></Guarded> },
      { path: "sources/new", element: <Guarded><CreateSource /></Guarded> },
      { path: "sources/:sourceId", element: <Guarded><SourceDetail /></Guarded> },
      { path: "sources/:sourceId/edit", element: <Guarded><EditSource /></Guarded> },

      // Composers
      { path: "composers", element: <Guarded><Composers /></Guarded> },
      { path: "composers/new", element: <Guarded><CreateComposer /></Guarded> },
      { path: "composers/:composerId", element: <Guarded><ComposerDetail /></Guarded> },

      // Streams
      { path: "streams", element: <Guarded><Streams /></Guarded> },
      { path: "streams/new", element: <Guarded><CreateStream /></Guarded> },
      { path: "streams/:streamId", element: <Guarded><StreamDetail /></Guarded> },
      { path: "streams/:streamId/edit", element: <Guarded><EditStream /></Guarded> },

      // Logs
      { path: "logs", element: <Guarded><Logs /></Guarded> },

      {
        path: "*",
        element: <Navigate to="/streams" replace />,
      },
    ],
  },
]);

document.addEventListener("DOMContentLoaded", () => {
  const isDarkMode = document.documentElement.classList.contains("dark");

  ReactDOM.createRoot(document.getElementById("root")!).render(
    <>
      <RouterProvider router={router} />
      <Toaster
        toastOptions={{
          style: {
            background: isDarkMode ? "#1f2937" : "#ffffff",
            color: isDarkMode ? "#f9fafb" : "#111827",
            border: "none",
            borderRadius: "0.125rem",
            boxShadow: "0 1px 2px 0 rgb(0 0 0 / 0.05)",
          },
        }}
        position="top-center"
      />
    </>,
  );
});

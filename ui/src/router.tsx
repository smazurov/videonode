import { lazy } from "react";
import { createBrowserRouter, Navigate } from "react-router-dom";

import Root from "./root";
import LoginRoute from "./routes/login";
import VideoRoute from "./routes/video";
import ErrorBoundary from "./components/ErrorBoundary";
import { Guarded } from "./components/RouteGuards";

const routes = {
  Sources: lazy(() => import("./routes/sources")),
  CreateSource: lazy(() => import("./routes/create-source")),
  SourceDetail: lazy(() => import("./routes/source-detail")),
  EditSource: lazy(() => import("./routes/edit-source")),

  Composers: lazy(() => import("./routes/composers")),
  CreateComposer: lazy(() => import("./routes/create-composer")),
  ComposerDetail: lazy(() => import("./routes/composer-detail")),

  Streams: lazy(() => import("./routes/streams")),
  CreateStream: lazy(() => import("./routes/create-stream")),
  StreamDetail: lazy(() => import("./routes/stream-detail")),
  EditStream: lazy(() => import("./routes/edit-stream")),

  Recordings: lazy(() => import("./routes/recordings")),
  RecordingDetail: lazy(() => import("./routes/recording-detail")),

  Logs: lazy(() => import("./routes/logs")),
};

export const router = createBrowserRouter([
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
      {
        path: "sources",
        element: (
          <Guarded>
            <routes.Sources />
          </Guarded>
        ),
      },
      {
        path: "sources/new",
        element: (
          <Guarded>
            <routes.CreateSource />
          </Guarded>
        ),
      },
      {
        path: "sources/:sourceId",
        element: (
          <Guarded>
            <routes.SourceDetail />
          </Guarded>
        ),
      },
      {
        path: "sources/:sourceId/edit",
        element: (
          <Guarded>
            <routes.EditSource />
          </Guarded>
        ),
      },

      // Composers
      {
        path: "composers",
        element: (
          <Guarded>
            <routes.Composers />
          </Guarded>
        ),
      },
      {
        path: "composers/new",
        element: (
          <Guarded>
            <routes.CreateComposer />
          </Guarded>
        ),
      },
      {
        path: "composers/:composerId",
        element: (
          <Guarded>
            <routes.ComposerDetail />
          </Guarded>
        ),
      },

      // Streams
      {
        path: "streams",
        element: (
          <Guarded>
            <routes.Streams />
          </Guarded>
        ),
      },
      {
        path: "streams/new",
        element: (
          <Guarded>
            <routes.CreateStream />
          </Guarded>
        ),
      },
      {
        path: "streams/:streamId",
        element: (
          <Guarded>
            <routes.StreamDetail />
          </Guarded>
        ),
      },
      {
        path: "streams/:streamId/edit",
        element: (
          <Guarded>
            <routes.EditStream />
          </Guarded>
        ),
      },

      // Recordings
      {
        path: "recordings",
        element: (
          <Guarded>
            <routes.Recordings />
          </Guarded>
        ),
      },
      {
        path: "recordings/:streamId/:session",
        element: (
          <Guarded>
            <routes.RecordingDetail />
          </Guarded>
        ),
      },

      // Logs
      {
        path: "logs",
        element: (
          <Guarded>
            <routes.Logs />
          </Guarded>
        ),
      },

      {
        path: "*",
        element: <Navigate to="/streams" replace />,
      },
    ],
  },
]);

import ReactDOM from "react-dom/client";
import "./index.css";
import { RouterProvider } from "react-router-dom";
import { Toaster } from "react-hot-toast";

import { router } from "./router";

document.addEventListener("DOMContentLoaded", () => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <>
      <RouterProvider router={router} />
      <Toaster
        toastOptions={{
          style: {
            background: "var(--color-surface-raised)",
            color: "var(--color-fg)",
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

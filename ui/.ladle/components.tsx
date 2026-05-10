import type { GlobalProvider } from "@ladle/react";
import { useEffect } from "react";
import "./styles.css";

// Sync Ladle's built-in theme toggle to our .dark class on <html>, which is
// what our semantic tokens expect (see src/design/tokens.css).
export const Provider: GlobalProvider = ({ children, globalState }) => {
  useEffect(() => {
    const root = document.documentElement;
    if (globalState.theme === "dark") {
      root.classList.add("dark");
    } else {
      root.classList.remove("dark");
    }
  }, [globalState.theme]);

  return (
    <div className="min-h-screen bg-surface text-fg p-6">
      {children}
    </div>
  );
};

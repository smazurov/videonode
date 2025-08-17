import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import tailwindcss from "@tailwindcss/vite";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig(({ mode, command }) => {
  const plugins = [
    tailwindcss(),
    tsconfigPaths(),
    react()
  ];

  return {
    plugins,
    build: { outDir: "dist" },
    server: {
      host: "localhost",
      port: 3000,
      proxy: {
        "/api": "http://localhost:8090"
      }
    }
  };
});

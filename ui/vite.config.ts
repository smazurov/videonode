import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import tailwindcss from "@tailwindcss/vite";
import tsconfigPaths from "vite-tsconfig-paths";
import { vitePluginVersionMark } from "vite-plugin-version-mark";

export default defineConfig(({ mode, command }) => {
  const plugins = [
    tailwindcss(),
    tsconfigPaths(),
    react(),
    vitePluginVersionMark({
      name: 'videonode-ui',
      command: {
        commands: [
          'git rev-parse --short HEAD',
          'date -u +"%Y-%m-%d %H:%M"'
        ],
        separator: ' • '
      },
      ifMeta: false,
      ifLog: false,
      ifGlobal: true
    })
  ];

  return {
    plugins,
    build: { 
      outDir: "dist",
      chunkSizeWarningLimit: 1000
    },
    server: {
      host: "localhost",
      port: 3000,
      proxy: {
        "/api": "http://localhost:8090"
      }
    }
  };
});

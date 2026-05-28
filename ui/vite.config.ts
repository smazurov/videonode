import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import tsconfigPaths from "vite-tsconfig-paths";
import { vitePluginVersionMark } from "vite-plugin-version-mark";

export default defineConfig(() => {
  const plugins = [
    tailwindcss(),
    tsconfigPaths(),
    react({
      babel: {
        plugins: ["babel-plugin-react-compiler"],
      },
    }),
    vitePluginVersionMark({
      name: 'videonode-ui',
      command: {
        commands: [
          'git describe --tags --always --dirty',
          'date -u +"%Y-%m-%dT%H:%M:%SZ"'
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
    cacheDir: "node_modules/.vite-app",
    build: {
      outDir: "dist",
      chunkSizeWarningLimit: 1000
    },
    server: {
      host: "localhost",
      port: parseInt(process.env.VITE_DEV_PORT || "5173", 10),
      proxy: {
        "/api": `http://localhost${process.env.VIDEONODE_SERVER_PORT || ":8090"}`,
        "/openapi.json": `http://localhost${process.env.VIDEONODE_SERVER_PORT || ":8090"}`
      }
    }
  };
});

import { defineConfig } from "vitest/config";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import { vitePluginVersionMark } from "vite-plugin-version-mark";

export default defineConfig(() => {
  const plugins = [
    tailwindcss(),
    react(),
    babel({ presets: [reactCompilerPreset()] }),
    vitePluginVersionMark({
      name: 'videonode-ui',
      command: {
        commands: [
          'echo ${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}',
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
    resolve: {
      tsconfigPaths: true
    },
    test: {
      // Unit tests live in src/; tests/ is the Playwright e2e dir (own runner).
      include: ["src/**/*.{test,spec}.{ts,tsx}"],
    },
    build: {
      outDir: "dist",
      chunkSizeWarningLimit: 1000
    },
    server: {
      host: "127.0.0.1",
      port: parseInt(process.env.VITE_DEV_PORT || "5173", 10),
      proxy: {
        "/api": `http://localhost${process.env.VIDEONODE_SERVER_PORT || ":8090"}`,
        "/openapi.json": `http://localhost${process.env.VIDEONODE_SERVER_PORT || ":8090"}`,
        "/whep": `http://localhost${process.env.VIDEONODE_SERVER_PORT || ":8090"}`
      }
    }
  };
});

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// During development the API, its WebSocket stream and the mock provider's
// admin API are proxied, so the console always talks to its own origin, as
// it does behind nginx in Docker Compose.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8080", ws: true },
      "/mock": {
        target: "http://localhost:8090",
        rewrite: (path) => path.replace(/^\/mock/, ""),
      },
    },
  },
  build: {
    sourcemap: true,
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: { modules: { classNameStrategy: "non-scoped" } },
  },
});

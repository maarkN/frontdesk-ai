import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The SPA talks to the Go core-api under the same origin: in dev, Vite
// proxies /v1 to the local core-api (CORE_API_ADDR defaults to :8080).
// In production the static bundle is served behind the same host as /v1.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/v1": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});

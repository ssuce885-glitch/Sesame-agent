import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 4173,
    proxy: {
      "/v2": {
        target: process.env.VITE_SESAME_ORIGIN ?? "http://127.0.0.1:8421",
        changeOrigin: true,
        selfHandleResponse: false,
      },
      "/v1": {
        target: process.env.VITE_SESAME_ORIGIN ?? "http://127.0.0.1:4317",
        changeOrigin: true,
        selfHandleResponse: false,
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
  },
});

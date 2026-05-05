import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) {
            return undefined;
          }
          if (
            id.includes("/react/") ||
            id.includes("/react-dom/") ||
            id.includes("/scheduler/")
          ) {
            return "vendor-react";
          }
          if (id.includes("/react-router") || id.includes("/@remix-run/")) {
            return "vendor-router";
          }
          if (
            id.includes("/@tanstack/react-query/") ||
            id.includes("/@tanstack/query-core/")
          ) {
            return "vendor-query";
          }
          if (
            id.includes("/react-syntax-highlighter/") ||
            id.includes("/refractor/") ||
            id.includes("/prismjs/")
          ) {
            return "vendor-syntax";
          }
          if (
            id.includes("/react-markdown/") ||
            id.includes("/remark-") ||
            id.includes("/micromark") ||
            id.includes("/unified/") ||
            id.includes("/mdast-util") ||
            id.includes("/hast-util") ||
            id.includes("/unist-util") ||
            id.includes("/vfile")
          ) {
            return "vendor-markdown";
          }
          if (
            id.includes("/recharts/") ||
            id.includes("/d3-") ||
            id.includes("/victory-vendor/")
          ) {
            return "vendor-charts";
          }
          return undefined;
        },
      },
    },
  },
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

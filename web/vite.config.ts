import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// `npm run dev` serves the UI with hot reload and proxies the API to the Go server on :3000.
export default defineConfig({
  plugins: [react()],
  server: { port: 5173, proxy: { "/api": "http://localhost:3000" } },
  // Never inline assets as data: URIs: the CSP only allows fonts and scripts from our own origin.
  build: { outDir: "dist", sourcemap: false, assetsInlineLimit: 0 },
});

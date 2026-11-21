import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The dev server proxies the API to a locally running breeferd (dry-run mode
// is fine) so the UI can be developed without a real hosting server.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    port: 5173,
    proxy: { "/api": { target: "http://127.0.0.1:2087", changeOrigin: false } },
  },
  build: { outDir: "dist", sourcemap: false, target: "es2022" },
});

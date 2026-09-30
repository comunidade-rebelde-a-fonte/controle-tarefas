import { fileURLToPath, URL } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Alvo do proxy /api. No docker compose: http://api:<HTTP_PORT>; no host: localhost.
const apiTarget = process.env.VITE_API_PROXY_TARGET ?? `http://localhost:${process.env.HTTP_PORT ?? "8080"}`;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  server: {
    port: Number(process.env.VITE_PORT ?? 5173),
    strictPort: true,
    // Bind mount no macOS não propaga eventos de arquivo: polling no container.
    watch: process.env.VITE_POLLING === "true" ? { usePolling: true, interval: 300 } : undefined,
    proxy: {
      "/api": { target: apiTarget, changeOrigin: false },
    },
  },
  preview: {
    port: Number(process.env.VITE_PORT ?? 4173),
    strictPort: true,
    proxy: {
      "/api": { target: apiTarget, changeOrigin: false },
    },
  },
});

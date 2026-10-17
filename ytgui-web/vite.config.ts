import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev: open http://localhost:5173/?token=<token printed by the backend>.
// The Go backend's dev mode accepts Origin http://localhost:5173.
export default defineConfig({
  base: "/",
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8765",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});

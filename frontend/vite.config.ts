import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Read the build-time env flag without pulling in @types/node.
declare const process: { env: Record<string, string | undefined> };

export default defineConfig({
  // Relative paths only for the webview embed; standalone dac keeps base "/".
  base: process.env.DAC_EMBED ? "./" : "/",
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api": "http://localhost:8321",
    },
  },
});

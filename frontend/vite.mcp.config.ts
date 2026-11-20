import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { viteSingleFile } from "vite-plugin-singlefile";

// Bruin Cloud's dashboard viewer for Claude (MCP Apps): one self-contained HTML page,
// attached to each release as dac-mcp-dashboard.html and served by Cloud's MCP server.
export default defineConfig({
  root: "mcp",
  plugins: [react(), tailwindcss(), viteSingleFile()],
  build: {
    outDir: "../dist-mcp",
    emptyOutDir: true,
    rolldownOptions: { input: "mcp/dashboard.html" },
  },
});

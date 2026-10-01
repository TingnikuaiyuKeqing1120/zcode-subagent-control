import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import { fileURLToPath } from "node:url";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), wails("./bindings")],
  build: {
    rollupOptions: {
      // 双入口：主窗口 index.html + 悬浮面板 float.html
      input: {
        main: fileURLToPath(new URL("./index.html", import.meta.url)),
        float: fileURLToPath(new URL("./float.html", import.meta.url)),
      },
    },
  },
});

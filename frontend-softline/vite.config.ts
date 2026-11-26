import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

// /api проксируется на recsys-сервис, чтобы демо работало без CORS.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const recsysTarget = env.VITE_PROXY_RECSYS_TARGET || "http://localhost:8080";

  return {
    plugins: [react()],
    server: {
      port: 5173,
      // Большой дамп БД и посторонние каталоги не должны дёргать watcher.
      watch: { ignored: ["**/bd-for-test/**", "**/bd/**", "**/recommendations.sql"] },
      proxy: {
        // Демо по 05-DEMO: /api/v1 -> recsys (по умолчанию :8080).
        "/api/v1": {
          target: recsysTarget,
          changeOrigin: true,
        },
        // Старый клиент ЛК: /api/* -> /api/v1/* на recsys (бывший сервис на :8000).
        "/api": {
          target: recsysTarget,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, "/api/v1"),
        },
      },
    },
    preview: {
      port: 4173,
    },
  };
});

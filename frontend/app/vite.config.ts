import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

// Env files (yj-arch-core): required .env.local-dev plus optional .env.<environment> files.
// Loaded via Vite --mode (no plain .env).
// Platform run.* always exports values from .env.local-dev before RUN_COMMAND.

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const host = env.HOST || "127.0.0.1";
  const port = Number(env.PORT || "5173");

  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        "@": fileURLToPath(new URL("./src", import.meta.url)),
      },
    },
    define: {
      __APP_NAME__: JSON.stringify(env.NAME ?? ""),
      __APP_VERSION__: JSON.stringify(env.VERSION ?? ""),
    },
    server: {
      host,
      port,
      strictPort: true,
	  proxy: { "/api": "http://127.0.0.1:8080" },
    },
    preview: {
      host,
      port,
      strictPort: true,
    },
  };
});

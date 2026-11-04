import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, URL } from "node:url";

// Env files (yj-arch-core): required .env.local-dev plus optional .env.<environment> files.
// Loaded via Vite --mode from tools/secrets/<platform>/<app>/ (no plain .env).
// tools/runner exports values from .env.local-dev before calling run_one.

const appDir = path.dirname(fileURLToPath(import.meta.url));
const secretsDir = path.resolve(appDir, "../../tools/secrets", path.basename(path.dirname(appDir)), path.basename(appDir));
// Inside a docker build only the app folder is in context: fall back to it.
const envDir = existsSync(secretsDir) ? secretsDir : appDir;

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, envDir, "");
  const host = env.HOST || "127.0.0.1";
  const port = Number(env.PORT || "5173");

  return {
    envDir,
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

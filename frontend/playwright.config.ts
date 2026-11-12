import { fileURLToPath } from "node:url"
import { defineConfig } from "@playwright/test"
import { backend, dir, port } from "./e2e/env"

// The real backend binary (`make build`, after `npm run build` so the UI is embedded) runs on a
// throwaway data directory; the suite drives the embedded UI on the backend's own origin.
// Environment: LOOM_E2E_PORT (default 8080), LOOM_E2E_BIN (default ../loom), LOOM_E2E_DIR,
// PLAYWRIGHT_CHANNEL (default the system Chrome; CI may set another).
const repo = fileURLToPath(new URL("../", import.meta.url))
const bin = process.env.LOOM_E2E_BIN ?? `${repo}loom`

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./e2e/global-setup.ts",
  outputDir: `${dir}/results`,
  workers: 1,
  fullyParallel: false,
  reporter: [["list"]],
  use: {
    baseURL: backend,
    headless: true,
    channel: process.env.PLAYWRIGHT_CHANNEL ?? "chrome",
  },
  webServer: {
    command: `rm -rf '${dir}' && mkdir -p '${dir}/data' && printf 'httphost:\\n  addr: "127.0.0.1:${port}"\\n' > '${dir}/substrate.yaml' && HOME='${dir}/profile' XDG_DATA_HOME='${dir}/profile/data' XDG_CONFIG_HOME='${dir}/profile/config' XDG_STATE_HOME='${dir}/profile/state' SUBSTRATE_CONFIG='${dir}/substrate.yaml' exec '${bin}' serve --no-project -db '${dir}/data/loom.db'`,
    url: `${backend}/healthz`,
    // Never reuse a server: this suite owns the data directory it starts on.
    reuseExistingServer: false,
    timeout: 60_000,
  },
})

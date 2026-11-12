import { fileURLToPath } from "node:url"
import { defineConfig } from "@playwright/test"
import { backend, dir, freshBackend, freshDir, freshPort, port } from "./e2e/env"

// The real backend binary (`make build`, after `npm run build` so the UI is embedded) runs on a
// throwaway data directory; the suite drives the embedded UI on the backend's own origin.
// Environment: LOOM_E2E_PORT (default 8080), LOOM_E2E_BIN (default ../loom), LOOM_E2E_DIR,
// PLAYWRIGHT_CHANNEL (default the system Chrome; CI may set another).
const repo = fileURLToPath(new URL("../", import.meta.url))
const bin = process.env.LOOM_E2E_BIN ?? `${repo}loom`

/** Wipes `where`, optionally creates the owner with the CLI, then runs the server in the foreground. */
function server(where: string, listen: string, withOwner: boolean) {
  const env = `HOME='${where}/profile' XDG_DATA_HOME='${where}/profile/data' XDG_CONFIG_HOME='${where}/profile/config' XDG_STATE_HOME='${where}/profile/state' SUBSTRATE_CONFIG='${where}/substrate.yaml'`
  const run = `${env} '${bin}' --no-project -db '${where}/data/loom.db'`
  const owner = withOwner
    ? ` && head -c 18 /dev/urandom | base64 | tr '/+' '-_' | tr -d '=\\n' > '${where}/owner-password' && ${run} identity setup --username owner --display-name 'E2E Owner' < '${where}/owner-password'`
    : ""
  return `rm -rf '${where}' && mkdir -p '${where}/data' && printf 'httphost:\\n  addr: "127.0.0.1:${listen}"\\n' > '${where}/substrate.yaml'${owner} && exec env ${env} '${bin}' --no-project -db '${where}/data/loom.db' serve`
}

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
  webServer: [
    {
      // The owner is created on the host, as in production (first-run setup is closed over HTTP):
      // `identity setup` runs before the server starts, with the password on stdin from a file
      // only the test reads.
      command: server(dir, port, true),
      url: `${backend}/healthz`,
      // Never reuse a server: this suite owns the data directory it starts on.
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      // A backend with no owner at all: the screen that says how to create one.
      command: server(freshDir, freshPort, false),
      url: `${freshBackend}/healthz`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
})

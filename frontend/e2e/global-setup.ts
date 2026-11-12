import { randomBytes } from "node:crypto"
import { writeFileSync } from "node:fs"
import { request } from "@playwright/test"
import { authState, backend, ownerName, passwordFile } from "./env"

// First-run setup on the fresh backend, once for the whole run: creates the owner and keeps the
// session cookie in storageState. Writes carry X-Requested-With and a same-origin Origin.
export default async function globalSetup() {
  const api = await request.newContext({
    baseURL: backend,
    extraHTTPHeaders: { "X-Requested-With": "loom-e2e", Origin: backend },
  })
  const state = await (await api.get("/api/setup")).json()
  if (!state.setup_required) throw new Error("backend is not fresh: refusing to reuse a data dir")
  const password = randomBytes(18).toString("base64url")
  writeFileSync(passwordFile, password, { mode: 0o600 })
  const response = await api.post("/api/setup", {
    data: { username: ownerName, display_name: "E2E Owner", password },
  })
  if (!response.ok()) throw new Error(`e2e setup failed: ${response.status()}`)
  await api.storageState({ path: authState })
  await api.dispose()
}

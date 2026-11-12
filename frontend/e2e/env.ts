import { tmpdir } from "node:os"
import { join } from "node:path"

// Everything the suite needs from the environment, with the manifest's defaults as the fallback.
export const port = process.env.LOOM_E2E_PORT ?? "8080"
export const backend = `http://127.0.0.1:${port}`
/** A throwaway data directory; wiped when the backend starts. HOME and XDG live inside it too. */
export const dir = process.env.LOOM_E2E_DIR ?? join(tmpdir(), `loom-e2e-${port}`)
export const authState = join(dir, "owner-auth.json")
export const passwordFile = join(dir, "owner-password")
export const ownerName = "owner"

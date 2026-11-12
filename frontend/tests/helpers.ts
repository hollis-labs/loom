import { readFileSync } from "node:fs"
import { type APIRequestContext, type Page, request } from "@playwright/test"
import { authState, backend, ownerName, passwordFile } from "../e2e/env"

export const ownerPassword = () => readFileSync(passwordFile, "utf8")
export const signedOut = { cookies: [], origins: [] }
export const owner = authState

/** An API context holding the owner's session, for seeding that the UI has no screen for. */
export async function ownerApi(): Promise<APIRequestContext> {
  return request.newContext({
    baseURL: backend,
    storageState: authState,
    extraHTTPHeaders: { "X-Requested-With": "loom-e2e", Origin: backend },
  })
}

export async function signIn(page: Page, username: string, password: string) {
  await page.goto("/")
  await page.getByLabel("Username").fill(username)
  await page.getByLabel("Password").fill(password)
  await page.getByRole("button", { name: "Sign in" }).click()
}

export { ownerName }

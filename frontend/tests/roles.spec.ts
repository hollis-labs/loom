import { expect, test } from "@playwright/test"
import { owner, ownerApi, signIn } from "./helpers"

const password = "a second long passphrase"

test.beforeAll(async () => {
  const api = await ownerApi()
  for (const [username, scopes] of [
    ["reader", ["loom.read"]],
    ["writer", ["loom.read", "loom.write"]],
  ] as const) {
    const r = await api.post("/api/identity/users", {
      data: { username, display_name: username, password, scopes },
    })
    expect(r.status()).toBe(201)
  }
  await api.dispose()
})

test("the owner can list keys", async () => {
  const api = await ownerApi()
  const r = await api.get("/api/identity/keys")
  expect(r.status()).toBe(200)
  await api.dispose()
})

test.describe("without loom.write", () => {
  test.use({ storageState: { cookies: [], origins: [] } })
  test("a read-only user reads, and a compile shows the server's 403 in the UI", async ({
    page,
  }) => {
    await signIn(page, "reader", password)
    await expect(page.getByText("Seed bundle for Nanite wiki output")).toBeVisible()
    await page.goto("/jobs")
    await page.getByRole("button", { name: "Compile" }).click()
    await expect(page.getByText("Request failed")).toBeVisible()
    await expect(page.getByText(/loom\.write/)).toBeVisible()
  })
})

test.describe("without loom.admin", () => {
  test.use({ storageState: { cookies: [], origins: [] } })
  test("a non-admin is refused the key routes with scope_required", async ({ page }) => {
    await signIn(page, "writer", password)
    await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
    const r = await page.request.get("/api/identity/keys")
    expect(r.status()).toBe(403)
    expect((await r.json()).code).toBe("scope_required")
  })
})

test("owner state is still valid after the role tests", async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: owner })
  const page = await ctx.newPage()
  await page.goto("/")
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  await ctx.close()
})

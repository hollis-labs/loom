import { expect, test } from "@playwright/test"
import { ownerApi, ownerName, ownerPassword, signedOut, signIn } from "./helpers"

test.use({ storageState: signedOut })

test("signed out shows sign-in and makes no data call", async ({ page }) => {
  const calls: string[] = []
  page.on("request", (r) => {
    const { pathname } = new URL(r.url())
    if (pathname.startsWith("/api/")) calls.push(`${r.method()} ${pathname}`)
  })
  await page.goto("/")
  await expect(page.getByRole("heading", { name: "Sign in to Loom" })).toBeVisible()
  // The setup probe and the profile probe (401 when signed out) are the only calls; no data endpoint is touched.
  expect(calls.every((c) => c === "GET /api/setup" || c === "GET /api/identity/profile")).toBe(true)
})

test("a deep link signed out lands on sign-in", async ({ page }) => {
  await page.goto("/pages")
  await expect(page.getByRole("heading", { name: "Sign in to Loom" })).toBeVisible()
})

test("a wrong password is shown on the form and keeps the sign-in screen", async ({ page }) => {
  await signIn(page, ownerName, "not the right password")
  await expect(page.getByRole("alert")).toContainText("invalid username or password")
  await expect(page.getByRole("heading", { name: "Sign in to Loom" })).toBeVisible()
})

test("sign-in opens the shell, a reload keeps the session, sign-out returns", async ({ page }) => {
  await signIn(page, ownerName, ownerPassword())
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  await page.reload()
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  await page.getByRole("link", { name: "Sign out" }).click()
  await page.getByRole("button", { name: "Sign out" }).click()
  await expect(page.getByRole("heading", { name: "Sign in to Loom" })).toBeVisible()
})

test("the signed-in name survives a reload (from the profile route)", async ({ page }) => {
  await signIn(page, ownerName, ownerPassword())
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  await page.reload()
  await page.getByRole("link", { name: "Sign out" }).click()
  await expect(page.getByText("Signed in as E2E Owner")).toBeVisible()
})

test("too many wrong passwords: the 429 message says when to try again", async ({ page }) => {
  const api = await ownerApi()
  const made = await api.post("/api/identity/users", {
    data: {
      username: "limited",
      display_name: "Limited",
      password: "yet another long passphrase",
      scopes: ["loom.read"],
    },
  })
  expect(made.status()).toBe(201)
  await api.dispose()
  for (let i = 0; i < 6; i++) {
    await signIn(page, "limited", "wrong wrong wrong")
    await expect(page.getByRole("alert")).toBeVisible()
  }
  await expect(page.getByRole("alert")).toContainText(/try again in \d+ minute/i)
})

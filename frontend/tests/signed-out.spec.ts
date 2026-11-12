import { expect, test } from "@playwright/test"
import { ownerName, ownerPassword, signedOut, signIn } from "./helpers"

test.use({ storageState: signedOut })

test("signed out shows sign-in and makes no data call", async ({ page }) => {
  const calls: string[] = []
  page.on("request", (r) => {
    const { pathname } = new URL(r.url())
    if (pathname.startsWith("/api/")) calls.push(`${r.method()} ${pathname}`)
  })
  await page.goto("/")
  await expect(page.getByRole("heading", { name: "Sign in to Loom" })).toBeVisible()
  // The setup probe and the session probe are the only calls; no data endpoint is touched.
  expect(calls.every((c) => c === "GET /api/setup" || c === "GET /api/status")).toBe(true)
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

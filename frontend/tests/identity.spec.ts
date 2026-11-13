import { expect, test } from "@playwright/test"
import { owner } from "./helpers"

test.use({ storageState: owner })

test("account security: the change-password form is there", async ({ page }) => {
  await page.goto("/identity/account")
  await expect(page.getByRole("heading", { name: "Account security" })).toBeVisible()
  await expect(page.getByLabel("Current password")).toBeVisible()
  await expect(page.getByRole("button", { name: "Change password" })).toBeVisible()
})

test("account: the signed-in person survives a reload (kit reads the profile)", async ({
  page,
}) => {
  await page.goto("/identity/account")
  await page.reload()
  await expect(page.getByText("E2E Owner").first()).toBeVisible()
})

test("access keys: the owner creates a key, sees it once, and it is listed", async ({ page }) => {
  await page.goto("/identity/keys")
  await expect(page.getByRole("heading", { name: "Access keys" })).toBeVisible()
  await expect(page.getByText("No API tokens.")).toBeVisible()
  await page.getByRole("button", { name: "Create token" }).click()
  await page.getByLabel(/name/i).first().fill("e2e key")
  await page.getByRole("checkbox", { name: "loom.read" }).check()
  await page
    .getByRole("button", { name: /^create/i })
    .last()
    .click()
  await expect(page.getByText(/key_/).first()).toBeVisible()
})

test("active sessions: this session is listed", async ({ page }) => {
  await page.goto("/identity/sessions")
  await expect(page.getByText("This session", { exact: false })).toBeVisible()
})

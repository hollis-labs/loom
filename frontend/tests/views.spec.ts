import { expect, test } from "@playwright/test"
import { owner } from "./helpers"

// One fresh backend for the whole run, so these run in order: the empty states come before the
// writes that fill them.
test.describe.configure({ mode: "serial" })
test.use({ storageState: owner })

const nav = (page: import("@playwright/test").Page, name: string) =>
  page.getByRole("link", { name, exact: true }).click()

test("bundles: the seed bundle is listed", async ({ page }) => {
  await page.goto("/")
  await expect(page.getByText("Seed bundle for Nanite wiki output")).toBeVisible()
  await expect(page.getByText("nanite", { exact: true })).toBeVisible()
})

test("pages: empty state before anything is compiled", async ({ page }) => {
  await page.goto("/")
  await nav(page, "Pages")
  await expect(page.getByText("No pages yet")).toBeVisible()
})

test("compile jobs: compile, see the job complete, then the page", async ({ page }) => {
  await page.goto("/jobs")
  await page.getByLabel("Source material").fill("# E2E Page\n\nBody written by the browser suite.")
  await page.getByRole("button", { name: "Compile" }).click()
  await expect(page.getByRole("cell", { name: "completed" })).toBeVisible()
  await nav(page, "Pages")
  await expect(page.getByRole("heading", { name: "E2E Page" })).toBeVisible()
  await expect(page.getByText("Body written by the browser suite.").first()).toBeVisible()
})

test("pages: search narrows the list and an empty result shows the empty state", async ({
  page,
}) => {
  await page.goto("/pages")
  await expect(page.getByRole("heading", { name: "E2E Page" })).toBeVisible()
  await page.getByLabel("Search pages").fill("zzzz-no-such-page")
  await expect(page.getByText("No pages yet")).toBeVisible()
  await page.getByLabel("Search pages").fill("E2E")
  await expect(page.getByRole("heading", { name: "E2E Page" })).toBeVisible()
})

test("exports: export the bundle and list the files", async ({ page }) => {
  await page.goto("/exports")
  await page.getByRole("button", { name: "Export bundle" }).click()
  await expect(page.getByText("INDEX.md", { exact: false })).toBeVisible()
})

test("directives: parse shows the directives found", async ({ page }) => {
  await page.goto("/directives")
  await page.getByRole("button", { name: "Parse" }).click()
  await expect(page.getByText('"command": "note"')).toBeVisible()
})

test("a deep link serves the app, and an unknown path serves the shell with a 200", async ({
  page,
}) => {
  await page.goto("/jobs")
  await expect(page.getByRole("button", { name: "Compile" })).toBeVisible()
  const response = await page.goto("/no-such-page")
  expect(response?.status()).toBe(200)
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
})

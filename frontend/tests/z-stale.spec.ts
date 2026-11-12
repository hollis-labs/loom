import { expect, test } from "@playwright/test"
import { owner, ownerApi } from "./helpers"

test.use({ storageState: owner })

// Runs after the views spec (file order): it adds two pages of its own.
test("a slow answer for a superseded search does not overwrite the current list", async ({
  page,
}) => {
  const api = await ownerApi()
  for (const title of ["Stale Alpha", "Stale Beta"]) {
    const r = await api.post("/api/compile-jobs", {
      data: { bundle: "nanite", generator: "wiki_page", input: `# ${title}\n\nBody of ${title}.` },
    })
    expect(r.ok()).toBe(true)
  }
  await api.dispose()

  // Hold back the first list request (no query); answer the second (query "Beta") at once.
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  let held = false
  await page.route("**/api/pages?*", async (route) => {
    const q = new URL(route.request().url()).searchParams.get("q")
    if (q === "" && !held) {
      held = true
      await gate
    }
    await route.continue()
  })

  await page.goto("/pages")
  await page.getByLabel("Search pages").fill("Beta")
  await expect(page.getByRole("heading", { name: "Stale Beta" })).toBeVisible()
  await expect(page.getByRole("button", { name: /Stale Alpha/ })).toHaveCount(0)

  // Now the superseded response arrives last.
  const late = page.waitForResponse(
    (r) => r.url().includes("/api/pages?") && r.url().endsWith("q="),
  )
  release()
  await late
  await page.waitForTimeout(300)
  await expect(page.getByRole("heading", { name: "Stale Beta" })).toBeVisible()
  await expect(page.getByRole("button", { name: /Stale Alpha/ })).toHaveCount(0)
})

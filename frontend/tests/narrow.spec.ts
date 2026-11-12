import { expect, test } from "@playwright/test"
import { owner } from "./helpers"

// app-shell has no responsive nav (frontend/CONVERSION_NOTES.md): the 240px nav stays, so the
// content gets about 150px at 390px. The pages must still not overflow the viewport.
test.use({ storageState: owner, viewport: { width: 390, height: 800 } })

for (const path of ["/", "/pages", "/jobs", "/exports", "/directives", "/sign-out"]) {
  test(`390px wide: ${path} does not overflow horizontally`, async ({ page }) => {
    await page.goto(path)
    await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
    await page.waitForLoadState("networkidle")
    const width = await page.evaluate(() => document.documentElement.scrollWidth)
    expect(width).toBeLessThanOrEqual(390)
  })
}

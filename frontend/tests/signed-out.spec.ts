import { expect, test } from "@playwright/test"
import { ownerApi, ownerName, ownerPassword, signedOut, signIn } from "./helpers"

test.use({ storageState: signedOut })

const signInHeading = (page: import("@playwright/test").Page) =>
  page.getByRole("heading", { name: "Sign in" })

test("signed out shows sign-in and makes no data call", async ({ page }) => {
  const calls: string[] = []
  page.on("request", (r) => {
    const { pathname } = new URL(r.url())
    if (pathname.startsWith("/api/")) calls.push(`${r.method()} ${pathname}`)
  })
  await page.goto("/")
  await expect(signInHeading(page)).toBeVisible()
  // Only the identity kit's own probes (setup and session) run; no Loom data endpoint is touched.
  expect(calls.length).toBeGreaterThan(0)
  expect(calls.every((c) => /^GET \/api\/(setup|identity\/)/.test(c))).toBe(true)
})

test("a wrong password is shown on the form and keeps the sign-in screen", async ({ page }) => {
  await signIn(page, ownerName, "not the right password")
  await expect(page.getByRole("alert")).toContainText("invalid username or password")
  await expect(signInHeading(page)).toBeVisible()
})

test("sign-in opens the shell, a reload keeps the session, sign-out returns", async ({ page }) => {
  await signIn(page, ownerName, ownerPassword())
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  await page.reload()
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  await page.getByRole("link", { name: "Sign out" }).click()
  await page.getByRole("button", { name: "Sign out" }).click()
  await expect(signInHeading(page)).toBeVisible()
})

test("too many wrong passwords: the server's rate-limit message is shown", async ({ page }) => {
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
  await expect(page.getByRole("alert")).toContainText(/too many attempts/i)
})

// Expected failure until the kit shows the Retry-After wait (loom #25 had this; kit task CW-20261113-0030).
test("too many wrong passwords: the 429 message says when to try again", async ({ page }) => {
  test.fail(true, "CW-20261113-0030: kit-identity shows no Retry-After wait")
  const api = await ownerApi()
  const made = await api.post("/api/identity/users", {
    data: {
      username: "limited2",
      display_name: "Limited Two",
      password: "a long enough password 1",
      scopes: ["loom.read"],
    },
  })
  expect(made.status()).toBe(201)
  await api.dispose()
  for (let i = 0; i < 6; i++) {
    await signIn(page, "limited2", "wrong wrong wrong")
    await expect(page.getByRole("alert")).toBeVisible()
  }
  await expect(page.getByRole("alert")).toContainText(/try again in \d+ minute/i)
})

// Expected failure until the kit moves focus to the error (loom #25 had this; kit task CW-20261113-0029).
test("a failed sign-in moves focus to the error", async ({ page }) => {
  test.fail(
    true,
    "CW-20261113-0029: kit-identity leaves focus on the page body after a failed sign-in",
  )
  await page.goto("/")
  await page.getByLabel("Username").fill(ownerName)
  await page.getByLabel("Password").fill("wrong wrong wrong")
  await page.getByLabel("Password").press("Enter")
  await expect(page.getByRole("alert")).toBeFocused()
})

test("a signed-out deep link is kept through sign-in; an explicit sign-out goes home", async ({
  page,
}) => {
  await page.goto("/pages")
  await expect(signInHeading(page)).toBeVisible()
  await signInFields(page)
  await expect(page.getByLabel("Search pages")).toBeVisible()
  expect(new URL(page.url()).pathname).toBe("/pages")
  await page.getByRole("link", { name: "Sign out" }).click()
  await page.getByRole("button", { name: "Sign out" }).click()
  await signInFields(page)
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  expect(new URL(page.url()).pathname).toBe("/")
})

async function signInFields(page: import("@playwright/test").Page) {
  await page.getByLabel("Username").fill(ownerName)
  await page.getByLabel("Password").fill(ownerPassword())
  await page.getByRole("button", { name: "Sign in" }).click()
}

test("a session lost mid-use returns to sign-in with the kit's notice", async ({ page }) => {
  await signIn(page, ownerName, ownerPassword())
  await expect(page.getByRole("link", { name: "Compile jobs" })).toBeVisible()
  await page.context().clearCookies()
  await page.getByRole("link", { name: "Pages", exact: true }).click()
  await expect(page.getByText("Your session has ended")).toBeVisible()
  expect(new URL(page.url()).pathname).toBe("/pages")
})

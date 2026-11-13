# Frontend conversion notes: Loom onto Substrate

The backend moved onto Substrate (see `../docs/conversion-notes.md`). The frontend is moved onto app-shell and the
BOM-pinned kits. This file records what changed, what the framework did not cover, and what was verified.

## What each old view did (from `loom-previous/frontend/src`, the reference: the old app cannot be installed)

All five were one `DashboardPage` switched by in-memory state (no router); every call went through the sysop-ui
client with a tab-scoped bearer token. Every view fetched on mount and showed an `EmptyState` "Request failed" on any error.

| View | Reads | Writes | Notes |
|---|---|---|---|
| Bundles | `GET /api/bundles` | none | summary cards (count, "Canonical Store: SQLite") and one card per bundle (title, slug, description) |
| Pages | `GET /api/pages?bundle=nanite&q=` on mount and on each keystroke | none | search box, list of pages (title, summary or slug), selected page body in a `pre`; first page auto-selected; empty state "No pages yet" |
| Compile Jobs | `GET /api/compile-jobs` | `POST /api/compile-jobs {bundle: nanite, generator: wiki_page, input}` | textarea pre-filled with a stub page, Compile button, then reload of the table (ID, generator, status) |
| Exports | none | `POST /api/bundles/nanite/export {}` | "Export Nanite" button; shows the export dir and file names |
| Directives | none | `POST /api/directives/parse {text, source: ui, save: true}` | textarea pre-filled with sample directives, Parse button, raw JSON result or error. Note `save: true`: it records to the directive ledger |

The ported API serves all five (every route above is in the backend's table): no view is dropped and no Gap arises.

## What the frontend is now

- React 19, Vite 7, Tailwind 4, TypeScript 5.9: the same toolchain as Basalisk. Every `@hollis-labs/*` dependency is
  at the exact version in Substrate's `bom/bom.yaml` `kits:` and installs from the public npm registry.
  `@hollis-labs/sysop-ui` (a git dependency on a repository that does not exist) is gone.
- `ManifestShell` (app-shell) draws the navigation from `src/generated/frontend.manifest.json`, produced by
  `npm run manifest` (`substrate frontend`). The app's own nav items and surfaces are in `cmd/loom/ui/app.json`, named
  by `frontend.contributions` in the manifest; there is no client-side merge.
- Pages (`src/pages/`): Bundles, Pages, Compile jobs, Exports, Directives. Same five views as before, built from
  design-components (`Card`, `Metric`, `Table`, `Textarea`, `SearchInput`, `Pill`, `EmptyState`).
- Sign-in, the first-run (no owner) screen, account, access keys, active sessions and sign-out come from
  `@hollis-labs/kit-identity` (`IdentityProvider`, `IdentityGate`, `IDENTITY_PAGES`, `ApiKeysPage`). They are four
  `nav.footer` items in `cmd/loom/ui/app.json` (`/identity/account|keys|sessions|sign-out`). `src/identity/` is gone.
- `src/api/request.ts` is a small `fetch` wrapper: cookie session, `X-Requested-With` on every call, the flat
  `{code, message, details?}` error parsed into `ApiError`; any 401 raises `loom:unauthorized`, which `App.tsx`
  hands to the kit (`SessionLossBridge`) so it shows its "session expired" notice and returns to sign-in.
- Removed: `src/api/auth*` (the shared bearer token and its prompt), the sysop-ui API client, `auth.test.mjs`,
  `GET /api/health`.

## Behaviour differences

- The old UI hard-coded the `nanite` bundle for pages, compile and export. The three views now have a bundle picker
  over `GET /api/bundles`; the bundle named `nanite` is selected on first load when it exists (the old behaviour),
  otherwise the first. The Exports button reads "Export bundle" instead of "Export Nanite".
- First-run setup is closed over HTTP (Substrate d453ac8: `GET /api/setup` answers `{setup_required, setup_over_http}`,
  `POST /api/setup` is always `403 setup_offline`; Loom never sets `setup_over_http`). With no owner the UI shows a
  plain screen with no form: create the owner on the host with `loom identity setup --username NAME`, then sign in.
  The setup form is gone.
- Sign-in is the kit's. A signed-out deep link survives sign-in (the URL is kept). An explicit sign-out leaves the URL
  on `/identity/sign-out`; `initialPath()` in `App.tsx` replaces that with `/` so signing in again does not land on
  the sign-out page. The interim `/sign-out` path of the first port is gone (it now serves the unknown-path page);
  the old app had no router, so nothing outside Loom links to it.
- Loom gained account (change password), access keys and active sessions screens; the old app had none. Key scopes
  offered are `loom.read`, `loom.write`, `loom.mcp` (`scopeSuggestions`; the server has no scope catalogue and an
  owner's `*` is not grantable by name).
- Client URLs are real paths (`/pages`, `/jobs`, ...); the Go host's SPA fallback serves them.

## Gaps

1. **Dev proxy needs `identity.allowed_origins`.** Writes through the Vite dev server (`make ui-dev`) answer
   `403 bad_origin` ("the request did not come from this app"): the browser's `Origin` is the Vite origin, and the
   backend accepts only its own listener's origin. Verified in a real browser and with `curl`. Working setup (a
   backend setting, in the `SUBSTRATE_CONFIG` file used for development):
   `identity: { allowed_origins: ["http://127.0.0.1:<vite port>"] }`. The built, embedded UI is same-origin and needs
   nothing. Decision for the driver: document this in `docs/conversion-notes.md`, or ship a dev override.
2. **`GET /api/identity/profile`** (Substrate bac8c60) is read by kit-identity 0.1.1 on boot (the app no
   longer calls it): it answers the signed-in person or 401, so the Account page shows the person after a reload.
   (On the first branch head it was `route_not_mapped`; the UI probed `GET /api/status` until the pin moved.)
3. **No sign-out / user-menu region in app-shell**, so account and sign-out are `nav.footer` items (interim,
   Basalisk's pattern; CW-20261112-0107).
4. **kit-identity 0.1.3 (the BOM pin).** The profile after a reload (0.1.1), focus on the error after a failed sign-in
   (0.1.2) and the `Retry-After` wait on a 429 (0.1.3) are fixed in the kit and asserted by tests in a real browser. Still open:
   the sessions page shows its title twice (shell frame plus page heading; CW-20261113-0031, waits on CW-20261112-0134).
   Not worked around.
5. **app-shell and design-bindings publish TypeScript source.** `tsconfig.json` needs `allowImportingTsExtensions`,
   Tailwind needs `@source "../node_modules/@hollis-labs/app-shell/src"` (app-shell has no `source.css`), and the dev
   server needs `optimizeDeps.esbuildOptions.jsx = "automatic"` or `ManifestShell` fails in dev with
   `React is not defined` (the production build is fine). All three are in this tree.
6. **`npm audit`** reports 8 findings (7 moderate, 1 high) in dev tooling (vite 7.3.6 dev server and transitive
   dev dependencies); the versions are the Basalisk pins and were not changed.
7. The Vite build prints "use client" warnings from `@base-ui/react`; harmless.
8. Biome 2.5.14 prints an info that `biome.json` is deprecated-shaped ("migrate"); same file as Basalisk.
9. **app-shell has no narrow-viewport layout.** At 390 px wide the left nav stays 240 px and leaves about 150 px for the
   page; nothing overflows horizontally (document width stays 390) but content is cramped. What a 390 px user sees:
   the full nav column on the left (about 240 px, with Sign out and Diagnostics at its foot), then a narrow column
   holding the page title, the bundle picker, the controls and (stacked, not side by side) the content, with long
   words wrapping and tables clipped. Checked on all five pages
   and sign-out in Chrome. The pages stack their own two-column layouts below 64rem; the shell is not worked around.
10. **`frontend.contributions` is in `cmd/loom/substrate.yaml`** (`ui/app.json`, relative to the manifest); the
    generated manifest comes from the real manifest (`npm run manifest`) and `substrate generate` leaves
    `substrate_wiring.go` unchanged.

## Browser tests

`npm run test:e2e` (Playwright, `tests/`) runs against the real backend binary on a throwaway data directory: the
config starts `../loom` (build with `npm run build` then `make build`) with HOME and the XDG directories inside the
temp directory, `rm -rf`s it first and never reuses a running server; global setup signs in and stores the
owner session. The suite covers sign-in/out, a wrong password, no data call while signed out, the five views with
empty states and each write, a real 403 error state (a read-only user compiling), the non-admin `scope_required` on
the key routes, deep links and 390 px width.

The owner is created on the host the way production does it: the server command runs `loom --no-project -db <db> identity setup`
(password on stdin from a file in the temp directory) before the server starts. A second backend (port + 1,
`LOOM_E2E_FRESH_PORT`) never gets an owner and backs the no-owner screen tests.

Identity coverage: account page, key create (one-time value shown, then listed), sessions, session lost mid-use,
deep link through sign-in, explicit sign-out.

Environment: `LOOM_E2E_PORT` (default 8080, the manifest default), `LOOM_E2E_BIN` (default `../loom`),
`LOOM_E2E_DIR` (default `$TMPDIR/loom-e2e-<port>`), `PLAYWRIGHT_CHANNEL` (default `chrome`, the system Chrome; no
browser is downloaded). CI needs: Node 24, Go, a system Chrome (or `PLAYWRIGHT_CHANNEL`), then `npm ci`,
`npm run typecheck`, `npm run lint`, `npm run build`, `make build`, `npm run test:e2e` in `frontend/`.

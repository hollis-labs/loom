# Frontend conversion notes: Loom onto Substrate

The backend moved onto Substrate (see `../docs/conversion-notes.md`). The frontend is the GUI track's
(`gui-loom`); this file records what the backend change means for it and the framework gaps found so far. Nothing
under `frontend/src` was changed by the backend port.

## Contract the backend now has

- **Errors** are `{code, message, details?}`; read `message`. Plain-text error bodies are gone.
- **401 means sign in.** The shared bearer token and its prompt (`src/api/auth*`, tab-scoped sessionStorage) no
  longer match anything: sign-in is a cookie session. First run: `GET /api/setup` (`setup_required`), `POST /api/setup
  {display_name, username, password}`; then `POST /api/login {username, password}`, `POST /api/logout`.
- **Writes** (every `POST` and `PUT`) send `X-Requested-With` (any non-empty value); the page's own `Origin` is
  required, and browsers add it.
- **Scopes:** no `loom.read` is 403 everywhere, no `loom.write` is 403 on changes.
- **Health** is `GET /healthz`; `GET /api/health` is gone.
- The paths and bodies of the other `/api/` routes are unchanged.

## Gaps

1. **`@hollis-labs/sysop-ui` is a git dependency** (`github:hollis-labs/sysop-ui#v0.9.0`, fetched over SSH).
   `npm ci` fails on the migration host ("Could not read from remote repository"), so typecheck and build could not
   be run here. The Substrate shape is `app-shell` and the exact-pinned kits in Substrate's `bom/bom.yaml` `kits:`,
   from the npm registry.
2. **No `lint` script** in `package.json`; CI runs typecheck and build only until one exists.
3. **The identity module has no UI** (Substrate guide section 8): setup, sign-in and password screens are the
   app's, and app-shell has no user-menu region, so sign-out is a page.
4. **`src/api/auth.test.mjs`** tests the removed bearer-token flow and goes with it.

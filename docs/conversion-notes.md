# Conversion notes: Loom onto Substrate

This records what changed when the Go backend moved onto Substrate, what the framework did not cover, and what an
operator, a client and the frontend need to do. It is a clean break for code and compatibility: no shims, no fallbacks. The data is not: Loom is deployed, so
every workspace database is migrated with no loss by `loom migrate-data` (below).

## Gaps

Things Substrate did not provide, or choices that need the owner. Each says what was done.

1. **Identity has no offline bootstrap, so a stdio-only `loom mcp` cannot get its key without a server. DECIDED by the
   owner; framework change pending.** In `enforce` mode `loom mcp` must present an API key (`mcp.stdio_token_file`),
   and a key can only be created through the identity module's HTTP API by a signed-in administrator. Loom's main
   agent use is `loom mcp` spawned by an MCP host against a project-local `.loom/` database, often with no server
   running. Today (option a, kept working exactly as the framework requires; the mcp module's token check is not
   weakened or bypassed): run `loom serve` once on that database, `POST /api/setup`, create a key with
   `POST /api/identity/keys`, save it to a file, point `mcp.stdio_token_file` at it.
   Owner decision: a stdio MCP process started locally against a local file runs under filesystem trust, the same as
   Loom's local CLI commands; enforced identity stays on the HTTP surfaces. The identity module also gets an offline
   bootstrap command (create the owner and a key from the command line, no server) for served instances. Both are
   framework changes routed to the Substrate Framework Agent; neither exists yet and Loom builds nothing ahead of
   them. **What Loom changes when they land:** no token file for local stdio (`mcp.stdio_token_file` is dropped from
   the launch config); a project-local database used only over stdio should not need identity tables or a signing
   key at all (expected, to confirm with the framework change), so no identity state is created beside it; served
   instances use the offline command for the first owner and keys; whether a gateway-launched child (Tether on
   the server host) counts as local is being decided with the Framework Agent.
2. **No shared Host check.** Guide section 5.7 asks the app to refuse Host headers that are not loopback or listed,
   to stop DNS rebinding, and to drop forwarding headers. Substrate has no module for it, so
   `internal/hostguard` is a copy of Basalisk's, with the flat error body and `loom.allowed_hosts`. Two copies of
   a security check is a smell: a Substrate `hostguard` option on `httphost` would replace both.
3. **Framework gaps closed at Substrate `09c04e5`.** Four things Basalisk worked around are now framework features and
   Loom uses them: the manifest carries the app's own `loom:` block (defaults in `substrate.yaml`, read from
   `app.Config["loom"]`); the `SUBSTRATE_CONFIG` override deep-merges (maps merge, scalars and lists replace, null
   removes a key), so the copy of the old one-level rule is gone; boot hooks (`boot.StartWith`): `AfterConfig`
   makes the database, identity key and secrets paths absolute against the data directory (`-db`, `-workspace`,
   `--no-project`, project-local `.loom/`, XDG; an absolute path from the operator's override is left alone) and
   `Before["database"]` runs the legacy-layout refusal reading the path from `app.Config`. `migrate-data` still runs
   before boot, as it builds the new database itself. Still true: the data options are pre-scanned from the command
   line because they decide the layout before any module starts.
4. **Frontend cannot be installed here.** `frontend/package.json` depends on `@hollis-labs/sysop-ui` as
   `github:hollis-labs/sysop-ui#v0.9.0` (a git dependency, fetched over SSH); `npm ci` fails with "Could not read
   from remote repository" on this host, so typecheck and build were not run. Moving to `app-shell` and the
   BOM-pinned kits (from the npm registry) is the GUI track; see `frontend/CONVERSION_NOTES.md`. CI builds the UI
   before `go test`; it will stay red until that lands.
5. **`scan-public --ref HEAD` is not clean, and part of it is in published history. DECIDED by the owner: the tree must be clean, history is left alone.** 16
   findings on the pre-conversion history plus tree (before any conversion commit):
   - tree and history, `internal/llm/redact_test.go`: an AWS key, a GitHub token, a JWT and a PEM header, invented
     inputs for the redaction tests (4 findings, +4 from gitleaks). **Fixed in the tree**: the test now builds them
     from parts at run time, so no secret-shaped literal is left (`--tree` is clean). They stay in history, where
     they are allowlist candidates: invented inputs for a redactor, not credentials;
   - history only: the commit message of `4f2be4b` names a build host; `.cerberus.yaml` (`d48dd40`)
     and `loom.cerberus.yaml` (`57fe127`) carry a macOS home path (`/Users/<name>`); `internal/server/security_test.go`
     (`ae90538`) carries a private IPv4 as a non-loopback test address.
   Rule (owner, every repo): the TREE is clean in the port PR (`scan-public --tree`, which CI and `make scan` now
   run); HISTORY is not rewritten during the port and gets one planned rewrite pass after the migration, when the
   `--ref HEAD` gate returns. The table above is kept for that pass. No allow file is needed for Loom.
6. **Pre-existing dead code, left alone.** `compiler.CompileWikiPage` is unreachable (it was already on main before
   the port). Not touched.
7. **OTel environment is hard-coded to `development`.** `newLoom` passes `Environment: "development"` to the OTel
   runtime, as the pre-port `main.go` did (twice). Carried over unchanged; not a setting yet.
8. **Not ported, not invented.** `loom directives compile` never existed as a CLI command (the dispatch path is HTTP
   and MCP only); it still does not. `wiki_links.relation` is still always `references`.

## Decisions made in the port

- **Accounts and keys come from the identity module.** The single shared bearer token and the tokenless loopback mode
  are gone. Even on loopback, every API and MCP request needs a signed-in session or a key. This is a posture change
  for a developer tool and it is the intended one.
- **Scopes:** `loom.read` (GETs), `loom.write` (POST and PUT, including compile, ingest, export and the
  parse/render calls that are POSTs), `loom.mcp` (the endpoint). Per tool, the mcp module's `Authorize` is called
  before every tool: a read-only tool (by its annotation) needs `loom.read`, any other `loom.write`, on top of
  `loom.mcp`. The table is built from `toolAnnotations`, so a new tool cannot be unmapped.
- **Anthropic key:** `secrets.env` is on in Loom's manifest, so the old environment variable keeps working as
  `loom.llm_api_key_ref: env://ANTHROPIC_API_KEY`; `keychain://`, `file://` and `op://` references work too.
- **MCP result cache owner** is the verified principal subject, no longer the `caller_id` argument. The argument
  existed only so clients could name themselves, and could be spoofed. A call with no verified principal is never
  cached and cannot fetch.
- **Local commands keep working on the database file** without HTTP, so without sign-in: they are filesystem-trust
  tools, as before.
- **Dropped config:** the `config.yaml` sections nothing read (`sources`, `drafts`, `nanite`, `ingest`).
- **Data is migrated, not reset** (owner ruling: every deployed app migrates). See "Data migration".

## Old to new

### Environment and flags

| Old | New |
|---|---|
| `serve -addr` (default `127.0.0.1:8080`) | `httphost.addr` (default `127.0.0.1:8080`) |
| `LOOM_API_TOKEN`, `serve -token` | removed: sign in, or send an API key |
| `LOOM_CORS_ORIGINS`, `serve -cors-origin` | removed: same-origin writes (identity); `identity.allowed_origins` for a proxy |
| `ANTHROPIC_API_KEY` | `loom.llm_api_key_ref: env://ANTHROPIC_API_KEY` (or a keychain/file/op reference) |
| `config.yaml` `llm.model` | `loom.llm_model` |
| `config.yaml` `paths.export_dir` | `loom.export_dir` (default `<data>/exports`); `paths.corpus_dir` and `media_dir` were unused |
| `config.yaml` `filters.redact_patterns`, `filters.git_exclude_paths` | `loom.redact_patterns`, `loom.git_exclude_paths` |
| `LOOM_OTEL_ENABLED`, `OTEL_ENABLED`, `serve/mcp -otel` | `loom.otel_enabled` |
| `LOOM_OTEL_METRICS_ENABLED`, `OTEL_METRICS_ENABLED`, `-otel-metrics` | `loom.otel_metrics` |
| `-otel-endpoint` | `loom.otel_endpoint` |
| `-db`, `-workspace`, `-project`, `--no-project` | unchanged |
| `loom migrate` | removed: the database module migrates the schema at start; `loom migrate-data` copies pre-Substrate data |
| `loom config` (merged config.yaml) | the skeleton's `loom config` (the merged Substrate config) |
| (new) | `SUBSTRATE_CONFIG`: YAML file holding any override above |

### Paths and files

| Old | New |
|---|---|
| `<data>/workspaces/<name>/main.db` (project-local `.loom/` or XDG) with `schema_version` | `<data>/workspaces/<name>/loom.db`, beside it; the old file is never written and stays as the rollback |
| (none) | `identity-signing.key` and its `.jwks.json`, and `secrets/`, in the data directory |
| `<config dir>/config.yaml`, `./config.yaml` | removed |

### HTTP

| Old | New |
|---|---|
| `GET /api/health` (open with a token) | `GET /healthz` `{status, checks}`, public |
| errors: plain text from `http.Error` | `{code, message, details?}`, with `code` such as `bad_request`, `not_found`, `unauthorized`, `forbidden` |
| `Authorization: Bearer <LOOM_API_TOKEN>` | a session cookie (browser) or `Authorization: Bearer key_...` / a minted token |
| every other `/api/` path | unchanged paths, methods and bodies |
| (new) | `/api/setup`, `/api/login`, `/api/logout`, `/api/password`, `/api/identity/*`, `/api/token`, `/api/pair` |

### MCP

| Old | New |
|---|---|
| tools take an optional `caller_id` | the argument is gone; the cache owner is the verified principal |
| `/mcp` needs the shared token | `/mcp` needs a verified credential holding `loom.mcp`; each tool also needs `loom.read` (read tools) or `loom.write` |
| `loom mcp` trusts its launcher | `loom mcp` needs `mcp.stdio_token_file` naming a key that holds `loom.mcp`, until local stdio trust lands (gap 1) |
| tool names, schemas (apart from `caller_id`), annotations | unchanged |

## For clients

Every caller of the old API changes how it authenticates; the paths and bodies stay.

- A router or agent (Fragments Engine's router, Nanite's Curator and Weaver, any MCP host): create an API key with
  scopes `loom.mcp loom.read loom.write` (drop `loom.write` for a read-only caller) and send it as `Authorization: Bearer key_...`. Stop sending `caller_id`.
- An MCP host that spawns `loom mcp`: add `mcp.stdio_token_file` to the `SUBSTRATE_CONFIG` file it launches Loom
  with, and keep passing `-db` (gap 1).
- The Sysop UI: see `frontend/CONVERSION_NOTES.md`.

## For the frontend

Everything the Sysop UI needs to build against, without asking the backend. Verified on a real binary of this branch
unless marked otherwise.

**Run it.** `make run` (or `go run ./cmd/loom serve --no-project`) with a `SUBSTRATE_CONFIG` file naming the port
(`httphost:` then `addr: "127.0.0.1:<port>"`; use an absolute path to the file). The default is `127.0.0.1:8080`;
`vite.config.ts` proxies `/api` to 8080, so change the proxy target if the port moves. `make ui-dev` runs Vite. With
`-db <file>` or `--no-project` the data (database, identity key, secrets) sits beside that file; without them it is
project-local `.loom/` or the XDG data directory. Use a temp directory and an explicit `-db` for development, never a
real data directory. Dev browser origin through the Vite proxy: writes are checked against the page's own `Origin`
and `Host`; the proxy forwards `Host` as the Vite host, which is loopback and passes the Host check (not yet tried in
a real browser). If a proxy changes the origin, list it in `identity.allowed_origins`.

**Sign-in flow (identity module, canonical paths, no `/api/identity/login`).**
- `GET /api/setup` (public) -> `{"setup_required": bool}`.
- `POST /api/setup {display_name, username, password}` (public) -> 200 profile + session cookie; `409 setup_closed`
  once an owner exists. The owner holds scope `*`.
- `POST /api/login {username, password}` (public) -> 200 profile + cookie; `401 invalid_login` (a bad credential: show it
  on the form, it is not "session ended"); `429 rate_limited` (five failures in five minutes for a user and address;
  the sixth answers 429 and so does the correct password until the window passes; no `Retry-After`, show "wait a few
  minutes").
- `POST /api/logout` -> 204, cookie cleared. `PUT /api/password {current_password, new_password}` -> 204 and every
  other session ends; wrong current password is `401 invalid_login` (not a signed-out state); `403 user_required`
  for an API key; `400 invalid_request` with the reason in `message`. Minimum password length is the server's
  (`identity.min_password`): show the server's message.
- Profile = `{id, username, display_name, email, scopes}`. There is no read route for the signed-in caller yet
  (`GET /api/identity/profile` answers 401/403 `route_not_mapped`; a framework addition is planned). After a reload the
  UI does not know the profile: treat any `401 unauthorized` as signed out.
- Also under `/api/identity/`: `PUT profile`, `GET sessions` and `DELETE sessions/{id}`, `GET|POST users` and
  `PUT users/{id}` (administrators: `loom.admin`), `GET|POST keys` and `DELETE keys/{id}`, `POST pairing-codes`,
  `GET jwks`; `POST /api/pair`, `POST /api/token` for machines. The cookie is HttpOnly, SameSite=Strict; the UI never
  sees it. Session life: the manifest sets 12h.
- Key creation `POST /api/identity/keys {name, scopes, audiences?, ttl_hours?}` -> 201 `{key: "key_...", credential}`;
  the key is shown once. `GET keys` -> `{"keys": [...]}`, and an empty list is `{"keys": null}` (treat null as empty);
  revoked keys vanish. The key, pairing-code and user routes need the app's admin scope, `loom.admin` (the app name plus `.admin`);
  the owner holds `*`, which covers it. Treat a 403 `scope_required` as "denied". (Per the identity contract in Torque
  CW-20261112-0080, comments 23312 and 23321; the `GET /api/identity/profile` route and the exact scope check on
  `keys` were not exercised on this branch.)

**Errors.** Every non-2xx `/api/` response is `{"code": "...", "message": "...", "details"?}`. Show `message`. Codes seen:
`unauthorized` 401 (no or bad credential: the signed-out signal), `invalid_login` 401, `scope_required` 403 (the
credential lacks the route's scope), `route_not_mapped` 403 (a path no scope table declares; fail closed), `csrf` 403 (a
cookie-authenticated write without `X-Requested-With`), `bad_origin` 403 (a write with a foreign `Origin`),
`user_required` 403, `rate_limited` 429, `setup_closed` / `exists` 409, `not_found` 404, `bad_request` /
`invalid_request` 400, `internal_error` 500. The Host check answers 403 with the same flat body for an unknown
`Host`. `/mcp` errors are the MCP endpoint's own shape (`{"error": {code, message}}`), not this one.

**Writes.** Every `POST`, `PUT` and `DELETE` made with the session cookie must send `X-Requested-With` (any non-empty
value, `XMLHttpRequest` by convention) and the browser's own `Origin` (browsers add it). A request with a bearer key
is not subject to that.

**Scopes.** `loom.read` for every GET, `loom.write` for every POST and PUT below, `loom.mcp` for `/mcp`. The first
owner has `*`. A signed-in user without a route's scope gets 403 `scope_required`; there is no UI for scopes beyond
what the identity routes return. API paths and bodies are unchanged from the old Loom apart from auth and errors.

| Method | Path | Scope |
|---|---|---|
| GET | `/api/status` | `loom.read` |
| GET, POST | `/api/bundles` | read, write |
| GET, PUT | `/api/bundles/{slug}` | read, write |
| GET | `/api/bundles/{slug}/pages`, `/links`, `/verifications`, `/conformance` | read |
| GET, POST | `/api/bundles/{slug}/compile-jobs` | read, write |
| POST | `/api/bundles/{slug}/export` | write |
| GET | `/api/pages` | read |
| GET | `/api/pages/{bundle}/{slug}`, `/links`, `/verifications`, `/conformance` | read |
| GET, POST | `/api/compile-jobs` | read, write |
| GET | `/api/compile-jobs/{id}`, `/api/compile-jobs/{id}/events` | read |
| GET, POST | `/api/ingest` | read, write |
| POST | `/api/ingest/text` | write |
| GET | `/api/ingest/{hash}` | read |
| GET | `/api/directives`, `/api/directives/{hash}` | read |
| POST | `/api/directives/parse`, `/api/directives/compile` | write (they are POSTs) |
| GET, POST | `/api/templates` | read, write |
| GET, PUT | `/api/templates/{name}` | read, write |
| POST | `/api/templates/{name}/render` | write (a POST) |

**`/mcp`.** The Streamable HTTP MCP endpoint, scope `loom.mcp`; each tool also needs `loom.read` (read-only tools) or
`loom.write`. A browser UI does not call it. `GET /healthz` is public and answers `{status, checks}`.

**How the built frontend is embedded and served.** `npm run build` writes to `internal/webui/dist/` (Vite's `outDir`),
which `internal/webui/embed.go` embeds with `//go:embed all:dist`; `Assets()` gives the `fs.FS` to
`httphost.Options{Assets}`. `dist/` is gitignored except `.gitkeep` (an empty tree compiles and serves nothing).
The host serves the files and falls back to `index.html` for any path that is not a file and not an API route
(client-side routing; an unknown `/x` returns the app shell with 200). The static files are served without a
credential, so the sign-in screen can load; every `/api/` route except `/api/setup`, `/api/login` and `/healthz`
requires one. Rebuild with `make ui-build` before `make build`; CI builds the frontend before `go build`.

**Frontend dependency problem.** `frontend/package.json` depends on `@hollis-labs/sysop-ui` as
`github:hollis-labs/sysop-ui#v0.9.0`, a git dependency fetched over SSH. `npm ci` failed on the migration host
("Could not read from remote repository"), so typecheck, lint and build were never run here. Whoever builds the
frontend needs a way to fetch it (SSH key for the host, or the package from the npm registry, which is the Substrate
shape: `app-shell` and the kits pinned in Substrate's `bom/bom.yaml`). See `frontend/CONVERSION_NOTES.md`.

**What the identity module does not provide:** screens (setup, sign-in, password, keys, pairing are the app's, or the
planned `@hollis-labs/kit-identity` kit), a user-menu region in app-shell, a profile read route, and a published
scope list (scopes are free strings, by convention `<app>.<verb>`).

## Data migration

Loom's data is one SQLite file per workspace (`<data>/workspaces/<name>/main.db`; `-db` names one directly). The
database module opens exactly one store per process, and Loom never has two workspaces open at once (a process
serves or runs one workspace, picked by `-workspace`), so the layout needs nothing from the framework: each
workspace maps to one run, as before. The only new rule is the file name.

- New schema file: `loom.db` beside the old `main.db` (an explicit `-db X` is used literally). Starting on a
  data directory whose `main.db` holds the old layout and has no `loom.db` stops with a message naming
  `loom migrate-data`; it never shows an empty Loom. An old-layout file reached any other way (`-db`,
  `SUBSTRATE_CONFIG`) is refused the same way.
- `loom migrate-data [--all] [--from OLD --to NEW]` (`cmd/loom/migrate.go`, `internal/storage/legacy.go`) runs
  before the modules start. It opens the old file read-only, requires all nine old migrations in `schema_version`,
  builds the new file under a temporary name with the goose schema, copies every table by column name inside one
  transaction (keys preserved; the new schema's seed rows are replaced by the old rows, so an edited or deleted
  seed stays as it was), compares per-table row counts, runs `integrity_check` and `foreign_key_check`, folds the
  WAL and only then renames the file into place. A failure leaves nothing at the target; a second run refuses
  because the target exists. The old file is not written, so rollback is: stop, point the previous build at the
  old data directory (`loom.db` can stay or go).
- Copied: `wiki_bundles`, `wiki_pages`, `wiki_links`, `wiki_verifications`, `compile_jobs`, `compile_events`,
  `directive_ledger`, `templates`, `ingest_ledger`. Not copied, on purpose: `wiki_result_cache` (a TTL cache keyed by
  the old caller-supplied `caller_id`, which no longer means anything). Accounts and keys are new (identity).
- Tests (`internal/storage/legacy_test.go`, `cmd/loom/boot_test.go`) use synthetic fixtures built from the nine
  old migrations (`internal/storage/testdata/legacy`): all tables, keys, links to pages, the `by` column, the
  old file byte-for-byte unchanged, the refusal paths and a real start on the migrated file.
- **Not yet done:** the rehearsal on a copy of the live files. It needs a copy made with the SQLite online backup
  (never put in the repo, a fixture, a log or a mail); row counts per table, integrity and foreign-key checks and the
  measured duration go here when it has run.

## The live instance and the cutover outline

What is known (from the owner side, not from touching the instance: it was not opened, copied or contacted):

- A deployed `loom serve --no-project` runs under a service manager on `127.0.0.1:8092`, and at least one
  `loom mcp` process is live against `~/.local/share/loom/workspaces/default/main.db`. The data directory is small
  (about 156 KB).
- What the code reads: `<data>/workspaces/<name>/main.db`; the user config `<config dir>/config.yaml` and a project
  `./config.yaml` (only `paths.export_dir`, `filters.*` and `llm.model` were consumed); the state directory holds the
  selected workspace; exports are written under `paths.export_dir`.
- Clients to list and change (the first two are confirmed by the owner side, the rest are what the README names
  and must be confirmed before cutover): HTTP callers of `:8092` (anything holding `LOOM_API_TOKEN`, anything
  relying on `LOOM_CORS_ORIGINS`); each MCP host config that spawns `loom mcp` (it now needs `mcp.stdio_token_file`
  and no `caller_id`); shell users of the CLI (unchanged except `migrate`/`config`); Fragments Engine's router and
  Nanite's Curator and Weaver, if they call Loom.

**Project-local databases.** The served `workspaces/<name>/main.db` files are not all of Loom's data: any project can
have its own `.loom/` directory (the CLI and `loom mcp` default to project-local paths), and agents use Loom over
stdio against those with no server running. "All data migrated" covers each of them. How they are met:

- Never silently and never on first sight: an old-layout `main.db` (workspace or `.loom/`) without a `loom.db`
  beside it makes every command, `loom mcp` included, stop at start with the message naming `loom migrate-data`.
  An agent launching `loom mcp` in an unmigrated project therefore sees the MCP server fail to start with that
  message on stderr and exit status 1 (the host reports a failed server); nothing is created or changed.
- The owner runs `loom migrate-data` in each project (it works on the project-local layout by the current
  directory, or `--from/--to`), or `--all` for the served workspaces. An inventory of the `.loom/` directories on the
  host is needed to plan the sweep: **asked of the driver**.
- Identity tables and the signing key live in the database and beside it, so a project-local database would carry
  its own owner, keys and signing key: one identity per project, or none. Whether stdio on a project-local file
  runs under filesystem trust (like the local commands) or must present a key is the owner's security-model
  question (Gap 1); nothing is built on either answer yet.

Outline, to be approved by the owner before any cutover:

1. **Data:** `loom migrate-data --all` on a copy first (the rehearsal above), then on the live files with the service
   stopped; then every project-local database from the inventory. Counts and checks must match; old `main.db`
   files stay.
2. **Clients:** per client, new credential (API key with `loom.read loom.write loom.mcp`), config change, owner:
   MCP hosts add `mcp.stdio_token_file`; HTTP callers send `Authorization: Bearer key_...`; stop sending
   `caller_id`. Run each against the new build before declaring done. Gap 1 (stdio bootstrap) is decided and waits on the framework
   change; until local trust lands a stdio host needs the token file.
3. **Config and supervision:** the service descriptor drops `--addr`, `LOOM_OTEL_ENABLED`, `ANTHROPIC_API_KEY`
   and gets `SUBSTRATE_CONFIG` (httphost.addr `127.0.0.1:8092`, `loom.otel_enabled`, `loom.llm_api_key_ref`), and the
   old `config.yaml` values move into the `loom:` block. Hand to the Cerberus owner.
4. **Deploy:** build in CI, install from GitHub (`fleet-install deploy loom --commit <sha>`, `--dry-run` first),
   keep the previous release, check `/healthz`.
5. **Rollback:** relink the previous release, restart it on the unchanged old data directory.
6. **Verify live:** first-run setup (owner), one write, one read through the real URL, an unknown `Host` refused,
   every MCP host listing tools.

## For the operator

- **Migrate the data before the first start:** `loom migrate-data` (or `--all`). Startup refuses an unmigrated
  old database and says so. First-run setup creates the owner afterwards.
- Health is `GET /healthz` (database included).
- `loom.cerberus.yaml` no longer carries `--addr`, `LOOM_OTEL_ENABLED` or `ANTHROPIC_API_KEY`: put
  `httphost.addr`, `loom.otel_enabled` and `loom.llm_api_key_ref` in the `SUBSTRATE_CONFIG` file the descriptor names.
  The descriptor belongs to Cerberus; the change in this repo's copy needs to reach the live one.
- Secrets: the `secrets/` directory and the signing key are private to the user. Losing the signing key ends minted
  tokens, not sessions or keys.
- Docker: the image has no baked-in token any more; mount a `SUBSTRATE_CONFIG` file that sets
  `httphost.addr: "0.0.0.0:8080"` (the default is loopback) and complete first-run setup.


## Rehearsal on a copy of the live file (2026-11-12)

Input: an SQLite online-backup copy of the live `workspaces/default/main.db` (quick_check ok, sha256 begins
8b689afe75feb54c), copied into a scratch directory with HOME and every XDG directory pointed at that directory.
No row content was copied anywhere else.

- Start on the old file: refused, message names `loom migrate-data`.
- `loom migrate-data --from main.db --to loom.db`: 14 ms. Per-table counts old/new: wiki_bundles 1/1, templates 1/1,
  wiki_pages, wiki_links, wiki_verifications, compile_jobs, compile_events, ingest_ledger, directive_ledger 0/0.
  Keys preserved (the bundle and template ids are the same). `integrity_check` ok, `foreign_key_check` clean.
- The old file's sha256 is unchanged after the run. A second run refuses (target exists). `loom status` on the new
  file reads the migrated data.
- `wiki_result_cache` is not copied: it is a TTL cache and repopulates on use (accepted by the driver).
- The served instance holds almost nothing, so this proves the mechanics, not volume.

## Test isolation

A manual run with an explicit `-db` and `--no-project` placed the identity signing key and `secrets` state under the
XDG default data directory instead of beside the database. Fixed: with `-db` they are placed beside that file.
Every test in `cmd/loom` runs with HOME and the XDG directories in a temp directory (`TestMain`), and a test fails if
the default layout resolves to the real home.

## Cutover: machines and clients

Two machines, two kinds of client.

- **The server host:** the user unit `loom serve --no-project --addr 127.0.0.1:8092` (no token in the unit; an
  environment file exists and was not opened). The caller of that port is not known. At cutover it needs the new
  binary, `loom migrate-data --all` on the stopped service, a first-run setup (owner) and API keys for each caller.
  The one stdio client that matters is the Tether gateway, which runs `loom mcp --no-project -db <workspace>/main.db`
  as a child to serve the loom tools to agents: under enforced auth its launch config needs `mcp.stdio_token_file`
  naming a file with an API key holding `loom.mcp` plus `loom.read` and/or `loom.write`, and the `-db` flag moves to
  the migrated `loom.db`. That is a Tether-side change, routed by the driver. How that key is first created without a
  server is Gap 1.
- **The owner's laptop:** locally launched agent sessions each run `loom mcp --no-project -db
  <repo>/.loom/data/workspaces/default/main.db` from an installed binary, and a second data directory exists there.
  Each of those project-local databases is a separate old database: migrate each with `loom migrate-data`, on that
  machine, then repoint the launch configs. An inventory of `.loom` directories on the server host found none.
- Tokens and CORS: the old shared API token holders (`LOOM_API_TOKEN` / `-token`) are replaced by per-caller API
  keys; there is no CORS setting to carry over.


## One write connection

`database.From(app)` is libs core/store: one write connection (`DB()`, `WriteTx`) and a separate read-only pool
(`Reader()`). Code written for an ordinary pool deadlocks if it issues a statement while a cursor or transaction
is open on the writer. Audit of `internal/storage`, `internal/service`, `internal/api` and the MCP tools: every
list method scans its rows into a slice and returns (one statement each, no nested query); the only transaction
(`UpsertPage`) uses `tx` throughout and calls no other repository method; the MCP result cache issues single
statements. No query is issued while another cursor is open. Change made anyway: the repository now writes through
the writer and reads through `Reader()` (`storage.NewRepositoryRW`), so a read never queues behind a write.
Tests: `TestEveryRouteWithOneReadConnection` and `TestEveryToolWithOneReadConnection` run every GET route (four
at once, beside writes) and every MCP tool against a store whose read pool is one connection, under a deadline.
(The database module has no read-pool setting, so Loom runs the default of four.)

# Loom Architecture

**Status:** implemented (Go rewrite). This document describes the system as it actually exists and runs today, not the original pre-implementation design proposal. Where the pilot integration with Fragments Engine and Nanite is still being verified end-to-end, that's called out explicitly rather than assumed.

Loom is the Go backend at `github.com/hollis-labs/loom` — a content-generation and wiki-compiler service. It replaces an earlier Python prototype; there is no dependency on that prototype's code or database anywhere in this system.

The backend is a [Substrate](https://github.com/hollis-labs/substrate) app: `cmd/loom/substrate.yaml` selects the framework modules, the generated `substrate_wiring.go` links them, and `internal/` is Loom's own code. Substrate supplies mechanism (config, logging, health, HTTP host, auth guard, identity, secrets, database and migrations, MCP plumbing, the CLI skeleton); Loom owns its data, its routes, its tools and its process. Loom is SQLite only (its SQL uses `?` placeholders and SQLite syntax).

---

## 0. The Substrate shape

### Layout

| Path | What |
|---|---|
| `cmd/loom/substrate.yaml` | The manifest: modules and their config. Edit it, then `make generate`. |
| `cmd/loom/substrate_wiring.go` | Generated. Do not edit. |
| `cmd/loom/main.go`, `config.go`, `serve.go`, `cli.go` | `main.go` boots the modules and builds the cobra root (`loom` / `loom serve` is the default; `loom mcp`; the flag-based local commands; the skeleton's `version`, `config`, `health`). `config.go` points the module config at the data directory and runs the legacy-layout guard. `serve.go` builds the handler chain. `cli.go` holds the local commands. |
| `internal/storage` | The one goose migration (the schema, `migrations/sqlite/00001_init.sql`), the repository, and the guard for the pre-Substrate layout. `storagetest` opens a migrated database for tests. |
| `internal/api` | The HTTP routes, their scopes (`Routes()`), flat errors. |
| `internal/mcp` | The MCP tools, registered on the mcp module's server (`RegisterTools`); `mcptest` builds a server for tests. |
| `internal/hostguard` | Host check (DNS rebinding) and forwarding-header hygiene. |
| `internal/settings` | The app's own `loom:` config block. |
| `internal/server` | Request body cap and OTel HTTP metrics around the host. |
| `internal/service`, `compiler`, `ingest`, `exporter`, `lint`, `wiki`, `directivex`, `llm` | The domain, unchanged by the port. |
| `internal/webui` | The embedded frontend (`Assets()`), built from `frontend/`. |

### Request path

```
client -> body cap / OTel -> hostguard -> identity.Middleware -> httphost
                                                                   |- auth guard (verifies the credential, enforces the route scope)
                                                                   |    |- /api/* identity routes (the module's)
                                                                   |    |- /api/* Loom routes (internal/api)
                                                                   |    |- /mcp (mcp module, scope loom.mcp)
                                                                   |- /healthz, and the SPA for everything else
```

### Auth model

- `auth.mode: enforce`. Every `/api/` route and `/mcp` needs a verified credential and the scope it declares. Nothing in the manifest or environment weakens it; only an explicit `SUBSTRATE_CONFIG` override can.
- The identity module is the issuer and the user store. First run: `GET /api/setup` reports `setup_required`, `POST /api/setup {display_name, username, password}` makes the owner (scope `*`), closes setup and signs in. The module also serves login, logout, password, profile, sessions, users, API keys, pairing codes, `/api/token` and `/api/identity/jwks` (Substrate `docs/identity.md`). A browser keeps the session cookie; sign-in lasts 12 hours.
- A cookie-authenticated write must carry an `X-Requested-With` header and a same-origin `Origin`. Requests with their own `Authorization` header (keys, tokens) are exempt.
- Clients that are not browsers (an agent, a router, Nanite's Curator) use an API key: an administrator creates one with `POST /api/identity/keys {name, scopes}` and the client sends it as `Authorization: Bearer key_...`.
- Single tenant. Loom has no per-user data: every caller with the scopes sees the same bundles and pages. The one per-caller thing is the MCP result cache, keyed by the verified principal subject (see §5).

### Scopes

Declared in one list (`api.Routes()`), from which the guard's scope table is built, so a route cannot be served without one; `scopes_test.go` proves the table.

| Scope | Routes |
|---|---|
| `loom.read` | every `GET` under `/api/` that Loom serves |
| `loom.write` | every `POST` and `PUT` that Loom serves (compile, ingest, bundle and template writes, export, directive parse/compile, template render) |
| `loom.mcp` | `/mcp` (HTTP) and the stdio server; each tool additionally needs `loom.read` (read-only tools) or `loom.write` (the rest), checked by the mcp module's `Authorize` |

The owner holds `*`. Identity's own routes carry `authenticated` or `loom.admin`. `/healthz` is public and answers only `{status, checks}`.

### Config

The manifest (`cmd/loom/substrate.yaml`) sets the defaults. A boot hook (`AfterConfig`) makes the three data paths absolute under the data directory once the override is applied; a path the override already made absolute is left alone. Any key can be overridden in a YAML file named by `SUBSTRATE_CONFIG`, which deep-merges: maps merge, scalars and lists replace, null removes a key.

| Key | Default | Meaning |
|---|---|---|
| `httphost.addr` | `127.0.0.1:8080` | Listen address. |
| `auth.mode` | `enforce` | Leave it. |
| `identity.key_file` | `<data>/identity-signing.key` | Token signing key. |
| `identity.session_ttl` | `12h` | Sign-in lifetime. |
| `identity.trusted_proxy`, `identity.allowed_origins`, `identity.secure_cookies` | see Substrate | For a local TLS proxy. |
| `secrets.state_dir` | `<data>/secrets` | Managed secret store. |
| `secrets.env` | `true` (manifest) | Allow `env://NAME` references, so `ANTHROPIC_API_KEY` becomes `loom.llm_api_key_ref: env://ANTHROPIC_API_KEY`. |
| `database.sqlite.path` | `<workspace dir>/loom.db` (`-db` is used literally) | Database file. |
| `mcp.scope` | `loom.mcp` | Scope `/mcp` needs. |
| `mcp.stdio_token_file` | none | File holding the API key `loom mcp` presents. Required in enforce mode. |
| `loom.allowed_hosts` | none | Host names served besides loopback, exactly as sent. Needs a loopback TLS proxy sending `X-Forwarded-Proto: https`. |
| `loom.export_dir` | `<data>/exports` | Where exports are written (confined: an export outside it is refused). |
| `loom.redact_patterns` | none | Regexps redacted from ingested text and from LLM-bound content. |
| `loom.git_exclude_paths` | `.git`, `node_modules`, `dist` | Path segments the file ingest skips. |
| `loom.llm_model` | `claude-sonnet-4-6` | Model for `generation_mode=llm` jobs. |
| `loom.llm_api_key_ref` | none | Secret reference to the Anthropic key; empty turns LLM generation off. |
| `loom.otel_enabled`, `loom.otel_metrics`, `loom.otel_endpoint` | off | OpenTelemetry export. |

### Data

One goose migration, `internal/storage/migrations/sqlite/00001_init.sql`, applied by the database module and including the two seed rows (the `nanite` bundle and the `wiki_page.default` template). Tables are in §2; the identity module adds `identity_*`. The file is `loom.db` beside the workspace's old `main.db`. Existing data is migrated by `loom migrate-data` (one-way, the old file untouched as the rollback); startup refuses an old-layout database and says so. Details, and the live instance and cutover outline, are in `conversion-notes.md`.

### What was dropped, and why

- `LOOM_API_TOKEN`, `-token`, `LOOM_CORS_ORIGINS`, `-cors-origin` and `internal/server`'s Protect: replaced by identity accounts and keys, the auth guard and `internal/hostguard`.
- `ANTHROPIC_API_KEY` read from the process environment by Loom: now `loom.llm_api_key_ref: env://ANTHROPIC_API_KEY` resolved by the secrets module.
- `-addr`: `httphost.addr`. `LOOM_OTEL_*` and `OTEL_*` environment switches and the `-otel*` flags: the `loom:` block.
- `GET /api/health`: `GET /healthz`.
- The `config.yaml` file (user and project) and `loom config`'s merged view: the `loom:` block; its `sources`, `drafts`, `nanite` and `ingest` sections were read by nothing and are gone. `loom config` is now the skeleton's.
- `loom migrate`: the database module migrates the schema at start; `loom migrate-data` copies old data.
- The hand-rolled migrator and its `schema_version` ledger; the nine numbered files, squashed (the old ones are kept only as test fixtures for `migrate-data`).
- `loom migrate` is replaced by `loom migrate-data`, which is a data copy, not a schema step.
- The `caller_id` tool argument (see §5).

---

## 1. What Loom actually does

Loom compiles raw material (a title, a body of text, a source reference) into a stored, structured **wiki page** — either deterministically (string-template substitution) or via a real LLM call (Anthropic). Every compiled page is OKF-shaped: it has frontmatter-equivalent structured fields (type, tags, status, provenance, trust) stored as real columns, not just synthesized at export time.

The wiki is one bundle-scoped concept inside a broader "compile fragments into structured output" job — Loom's compiler, directive dispatcher, and API surfaces are general-purpose; the wiki page generator (`generator: wiki_page`) is the one blueprint that's fully built out today.

Loom is a single Go binary exposing three parallel interfaces to the same underlying SQLite-backed service layer:
- **HTTP API** (`loom serve` — also hosts the embedded Sysop UI frontend and a `/mcp` endpoint)
- **CLI** (`loom <command>`)
- **MCP server** (`loom mcp`, stdio transport)

All three call into the same `internal/service.Compiler`/`internal/storage.Repository` — there's no logic duplicated per-interface.

---

## 2. Data model

SQLite, created by the one goose migration (`internal/storage/migrations/sqlite/00001_init.sql`). Core tables:

**`wiki_bundles`** — `slug` (unique), `title`, `description`, `scope` (`project|meta|personal`, validated), `okf_export_path`.

**`wiki_pages`** — `bundle_id`, `slug`, `path`, `type` (OKF's one required field, defaults `"note"`), `title`, `summary` / `description` (both kept — `description` defaults to `summary` when unset, not a rename), `tags` (json), `status` (defaults `"active"`) / `stale_after`, `generated_by` / `generated_at` (trust family — e.g. `loom-compiler` or `loom-compiler-llm`), `body`, `source` / `sources` (json array, `sources` defaults to `[source]`), `content_hash` (sha256 of body), `source_fragment_ids` (json array — provenance trail back to Fragments Engine's `fragments.id`, populated by whatever writes the page, not automatically).

**`wiki_links`** — real page-to-page relations: `from_page_id`, `to_page_id` (nullable — external URLs and unresolved internal targets stay null), `relation` (currently always `"references"` — no richer taxonomy yet), plus the original `target`/`kind`/`label`/`position` extracted-markdown-link fields.

**`wiki_verifications`** — `page_id`, `kind`/`status`/`message` (Loom's own automated structural/content checks, run at every `UpsertPage` — heading present, summary present, source present, link count), `by` (actor attestation family — distinct trust signal from the automated checks; `internal/wiki`'s own checks record `by: "loom-structural-checker"`, a real human/agent attestation would record something else).

**`wiki_result_cache`** — per-caller MCP tool result cache (id/caller_id/tool_name/expires_at/body), TTL-bound, used for the deep-dive-by-id mechanism on large `wiki_*` list/search tool results.

**`compile_jobs` / `compile_events`**, **`directive_ledger`**, **`ingest_ledger`**, **`templates`** — job tracking, directive parse/dispatch idempotency, file/text ingest dedup, and named compile templates (seeded: `wiki_page.default`).

---

## 3. The compile pipeline

Two paths, selected per-request via `generation_mode` (`""`/`"deterministic"` default, or `"llm"`):

- **Deterministic** (`compiler.CompileWikiPageWithTemplate`) — `strings.NewReplacer` substitution of `{{title}}/{{slug}}/{{summary}}/{{body}}/{{source}}` into a stored template body (`wiki_page.default`: `# {{title}}\n\n{{body}}`). No network calls. Used for tests, mechanical transforms, and any caller that wants byte-for-byte predictable output.
- **LLM** (`compiler.CompileWikiPageWithLLM`, `internal/llm` + `internal/llm/anthropic`) — a real Anthropic API call drafts the page body from the supplied title/context/source. Requires `loom.llm_api_key_ref` (a secret reference resolved by the secrets module); fails with a clear error if unset (deterministic path never needs one). Pre-send redaction runs against the body: a baseline set of secret-shaped regexes (API keys, AWS keys, bearer tokens, GitHub tokens, JWTs, PEM blocks) plus any caller-configured patterns (`loom.redact_patterns`, the same mechanism the file-ingest path uses).

Both paths converge on the same OKF-shaping step: `applyOKFDefaults` (mirrors `UpsertPage`'s own defaulting so a compiled-but-not-yet-persisted page is already fully shaped) and `RenderFrontmatter`, which renders a **flat** YAML frontmatter block (`path/type/title/description/tags/status/stale_after/generated_by/generated_at/sources/content_hash/source_fragment_ids` — matching `wiki_pages`' own column names directly, not the fully-nested OKF v0.2 spec shape). The result carries both the persisted `Page` and an in-memory-only `Document` (frontmatter + body) — `Document` never feeds `Page.Body` or `Page.ContentHash`, so a page's content hash stays stable across recompiles regardless of a changing `generated_at` timestamp.

**Confidence and diff** (`internal/service/confidence.go`): every compile-job response includes a `confidence` (0.0–1.0) and `confidence_tier` (`high`/`medium`/`low`), plus a `diff` summary when an existing page is being overwritten. No existing page → `1.0`/`high` (nothing to conflict with). Existing page, `content_hash` unchanged → `1.0`/`high` (no-op write). Existing page, body differs → confidence is an LCS-based line-similarity ratio between old and new body (`2·|LCS|/(len(old)+len(new))`, the same style `difflib.SequenceMatcher` uses), tiered `≥0.85` high / `≥0.50` medium / else low. The diff includes old/new `content_hash`, line counts and delta, the similarity ratio, and a self-contained unified-diff string. This is the signal external consumers (Nanite's Curator, see §6) are meant to use to decide "safe to write directly" vs. "stage for review" — it is not a heuristic Curator needs to reinvent.

---

## 4. Directives

`internal/directivex` wraps the shared `go-directives` library (inline `::command` markers). Parsing and ledgering (`directive_ledger`, idempotent by content hash) work independently of dispatch.

`internal/service/directives.go` dispatches each newly-ledgered directive by `Command` to a handler:
- **`draft`** — routes through the normal compile pipeline (template or LLM, per `::config generation_mode=llm`) — a draft directive *is* a compile job, just reached via explicit command instead of the implicit default.
- **`log-adr`** — compiles a page tagged `type: adr`, title/slug prefixed accordingly. Loom has no dedicated ADR store; this rides the same `wiki_pages` table.
- **`reminder`** — a deliberately minimal page (`type: reminder`, body is just the directive's own prompt text, no LLM pass, no template body) — "a lightweight record," not a compiled document.
- **`extract`** — body is the source lines `go-directives` itself attributed to the directive (`Directive.ContextRange`), not the whole document — verbatim extraction, not structured parsing.
- Anything else falls back to the original "compile a `wiki_page` from the full source text" behavior (unchanged from before dispatch existed).

Directives with no dedicated handler don't error; the ones that fail to even start a compile job (e.g. an unsupported `::config generator=...` override) are recorded in the response's `Failed` list rather than silently dropped.

---

## 5. API / CLI / MCP surfaces

All three surfaces expose materially the same operations. Highlights:

- **Compile**: `POST /api/compile-jobs`, `POST /api/directives/compile`, `loom compile`, `loom_compile_request`/`loom_compile_from_directives` MCP tools.
- **Conformance** (OKF §11 — every page has a `type` and parseable rendered frontmatter; `internal/lint`, deliberately separate from `internal/wiki`'s content-quality checks): `GET /api/pages/{bundle}/{slug}/conformance`, `GET /api/bundles/{bundle}/conformance`, `loom pages|bundles conformance`, `loom_page_conformance`/`loom_bundle_conformance`.
- **Export**: `POST /api/bundles/{bundle}/export`, `loom export` — regenerates a bundle's OKF-shaped directory (git-backed, Obsidian-browsable) from current `wiki_pages` rows via the same `compiler.RenderFrontmatter` the compile step uses, plus `## Links`/`## Verifications` audit sections and `index.md`/`log.md`. Nothing ever hand-edits the exported directory as a record of truth — it's regenerated, not written to.
- **MCP result cache**: 11 list/search tools are cache-wired (`internal/mcp/cache.go`) — a result whose full JSON exceeds a soft byte threshold gets cached (SQLite-backed, TTL-bound, scoped by the caller's verified principal subject) and a `wiki_result://<id>` pointer is added to the response hint; `loom_fetch_result`/`loom_search_result` retrieve the full result later. The owner is never a tool argument: it is the principal the auth guard verified (HTTP and stdio alike). A call with no verified principal is never cached and cannot fetch, since the cache table is a single shared store across processes — a shared bucket would let one unidentified client read another's cached data.

---

## 6. The pilot integration: Fragments Engine → Loom, via Nanite

This is the part of the design that's genuinely cross-repo, and where "implemented" and "verified working end-to-end" currently diverge. Read this section as status, not promise.

**The intended flow:**

1. **Capture** — raw material lands in Fragments Engine's existing inbox (chat logs, notes, files, URLs — no new capture surface).
2. **Tag** — FE's ingest pipeline runs `go-directives` over incoming content; anything containing an inline `::draft`/`::log-adr`/`::reminder`/`::extract` marker gets tagged with a `directive` entity.
3. **Route** — FE's rule-based router matches any directive-tagged fragment (deliberately broad; Curator does the classification, not FE) and dispatches to a `callback`-kind destination. Callback dispatch is **always async** — enqueued through FE's delivery queue, never attempted inline on the routing hot path.
4. **Wake** — the queued job fires a `POST` to Nanite's Curator wake endpoint with an opaque payload: `{"generator": "wiki_page", "fragment": {"id", "source", "source_type", "source_id", "title", "canonical_path"}}`. **No body content is included** — FE only sends an identifying pointer; the receiving agent fetches the fragment's content itself.
5. **Compile** — Curator (a Nanite durable agent, fresh session per wake) fetches the fragment's content, calls Loom's compile API, and uses the response's `confidence_tier`/`diff` to decide whether to write directly or stage the result for review.
6. **Recall** — a second durable agent, Weaver, answers questions against Loom's compiled pages using the `wiki_*` MCP tools, proposing updates back to Curator rather than writing directly.

**Confirmed working end to end, via live testing:**
- FE's directive tagging, routing, and async callback dispatch — a directive-tagged fragment reliably reaches Curator's wake endpoint.
- The wake mechanism itself — Curator wakes, gets a fresh session, and runs a real multi-turn conversation.
- Loom's compile API, confidence/diff signal, and MCP tool surface — independently verified (real LLM compiles, real confidence scoring on repeat compiles, real OTel traces).

**Not yet working:** Curator has not yet produced a compiled page from a live fragment. The open issue is in how Nanite's agent runtime resolves and calls tools during a durable-agent turn, not in anything Loom is responsible for. Weaver and Curator's scheduled lint+export tick are code-complete on the Nanite side but unverified end-to-end, pending the same fix.

---

## 7. Deployment

Loom runs as a single binary: `loom serve` hosts the HTTP API, the embedded UI, and `/mcp`; `loom mcp` runs the stdio MCP server. In the author's setup it runs as a local daemon managed by [Cerberus](https://github.com/hollis-labs/cerberus) (see `loom.cerberus.yaml`), with its settings in the `SUBSTRATE_CONFIG` file.

The CLI resolves project-local `.loom/` paths by default, so which database you get depends on the working directory. Any MCP host that spawns `loom mcp` from its own working directory should pass an explicit `-db` path pointing at the same SQLite store the HTTP daemon uses, and a `mcp.stdio_token_file` naming an API key.

The local commands (`compile`, `jobs`, `bundles`, `templates`, `pages`, `ingest`, `export`, `directives`, `status`) open the database file directly: no HTTP, no sign-in. Whoever can open the file can run them.

OTel tracing is opt-in (`loom.otel_enabled: true`) and exports over OTLP HTTP (for example to a local Jaeger collector at `127.0.0.1:4318`); traces show up under service name `loom`. Metrics export is opt-in separately (`loom.otel_metrics: true`) — Jaeger's OTLP receiver only implements traces, so point metrics at a collector that accepts them.

---

## 8. Known gaps

- **FE route fan-out** — one fragment can only match and fire one FE route today; a fragment that should become both a Loom wiki page and, say, a Torque task can't do both from a single route match yet.
- **OTel default polarity** — Loom defaults OTel off (opt-in) while Nanite defaults it on; there's no portfolio-wide convention yet.
- **`loom directives compile` CLI subcommand** — the directive-dispatch path is reachable via HTTP and MCP but not the CLI (`loom directives` only has `parse|list|get`).
- **The Curator tool-resolution gap** described in §6.

---

## 9. What this document deliberately does not cover

The original pre-implementation design (oriented around Karpathy's llm-wiki gist, OKF v0.2, and a from-scratch Curator/Weaver proposal) covered a broader vision — meta and personal wikis beyond the single `nanite` pilot bundle, multi-bundle query routing in Weaver, a Tesseract pointer-sync mirror, output fan-out across multiple generator types, non-text (infographic) generation. None of that is built, and none of it is implied by anything in this document. This document describes what runs today; treat anything not mentioned here as not yet built, not as an oversight.

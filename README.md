# Loom

Loom is a content-generation and wiki-compiler service. It takes raw material
— a title, a body, a source reference — and compiles it into a stored,
structured, OKF-shaped wiki page, either deterministically (template
substitution) or via a real LLM call (Anthropic). One SQLite-backed service
layer is exposed over HTTP, CLI, and MCP. Loom does not decide *what* to
compile, *when*, or with what title and provenance — that's the caller's job
(Fragments Engine's router, Nanite's Curator agent, or a person at the CLI).
It is not a capture inbox, an agent runtime, or a recall store.

> **Pre-release.** Loom is unreleased, not deployed, and has no outside
> consumers. It's being built in the open: the code, the docs, and this
> README describe what exists today, not a pitch for what's planned.
> Interfaces and behavior change without notice, and there are no
> compatibility guarantees yet.

## What it is today

- **OKF-shaped pages, for real.** Compiled pages carry structured fields
  (`type`, `tags`, `status`, `provenance`, `generated_by`/`generated_at`) as
  real columns, not just synthesized at export time.
- **Two compile paths.** Deterministic template substitution (no network
  call, byte-for-byte predictable) or an LLM pass via Anthropic, with
  pre-send redaction of secret-shaped content (API keys, tokens, PEM blocks)
  before anything leaves the process.
- **A confidence/diff signal on every compile job** — so a caller can tell a
  safe write from one that needs review, instead of reinventing that
  heuristic itself.
- **Conformance checking** (OKF §11) at the page and bundle level, plus
  structural verification (heading, summary, source, link count) on every
  write.
- **Directives** (`::draft`, `::log-adr`, `::reminder`, `::extract`) parsed
  and ledgered independently of dispatch, so a directive-tagged fragment
  becomes a compile job without a person triggering it by hand.
- **Regenerable export** to a git-backed, Obsidian-browsable directory — the
  exported tree is never hand-edited; it's rebuilt from the database.

## Where it sits in the stack

```
  capture / routing    Fragments Engine — inbox, directive tagging, async routing
         │
    ┌─────────┐
    │  Loom   │   compiles raw material → OKF-shaped wiki pages
    └─────────┘   HTTP API · CLI · MCP server, one shared service layer
         │
   consumers          Nanite's Curator (writes), Weaver (reads via wiki_* MCP
                       tools), any MCP host, a person at the CLI
```

Loom only compiles, stores, and serves. It never decides what's worth
compiling or routes anything on its own — that judgment stays upstream.

## Examples

**Direct use.** `loom compile < page.md` compiles and stores a page from
stdin; `loom pages conformance my-page` checks it against OKF §11 before
anyone treats it as done.

**Composition (the pilot in progress).** A note lands in Fragments Engine's
inbox and gets tagged with an inline `::draft` marker. FE's router dispatches
it, async, to Nanite's Curator agent. Curator fetches the fragment's content,
calls Loom's compile API, and uses the returned `confidence_tier`/`diff` to
decide whether to write the page directly or stage it for review. A second
agent, Weaver, later answers questions against the compiled pages through
Loom's `wiki_*` MCP tools, proposing updates back to Curator rather than
writing directly. See [`docs/architecture.md`](docs/architecture.md) §6 for
exactly what's confirmed working end-to-end versus still in progress.

## Roadmap

- **Finish the pilot integration end-to-end.** Loom's side (compile API,
  confidence signal, MCP surface) is independently verified; the open item is
  a tool-resolution issue in Nanite's agent runtime, not in Loom.
- **Beyond the wiki page.** The compiler, directive dispatcher, and API
  surfaces are already general-purpose — `wiki_page` is just the one
  generator blueprint that's fully built out today.
- **Richer link relations.** `wiki_links.relation` is currently always
  `"references"`; a real taxonomy is unbuilt.

## License

MIT — see [LICENSE](LICENSE).

## Prerequisites

- Go 1.26.6+
- Node.js 20+ / npm

## Build

```sh
make all      # ui-build (vite → internal/webui/dist) then build
./loom
```

The Sysop UI is then served at <http://localhost:8080/>.
Before the first `make ui-build`, `go-webui` serves a "not built"
placeholder in place of the app.

| Command | What it does |
|---|---|
| `make ui-build` | Build the frontend into `internal/webui/dist` |
| `make ui-dev` | Run the Vite dev server (hot reload) |
| `make build` | Build the Go binary |
| `make all` | `ui-build` then `build` |
| `make run` | Build and run the server |
| `make install` | Build the embedded UI + install `loom` to `$GOBIN` |
| `make test` / `make vet` | Go test / vet |
| `make lint` | `vet` plus golangci-lint/staticcheck/errcheck/govulncheck, each skipped if not installed |
| `make clean` | Remove the built binary and `frontend/node_modules` |

## Quick start

The same SQLite-backed Loom services are exposed through HTTP, CLI, and MCP:

```sh
loom serve                         # HTTP API, embedded UI, hosted /mcp
loom mcp                           # stdio MCP server
loom migrate                       # apply SQLite migrations
loom compile < page.md             # compile and store a wiki page (stdin, or -input "text/JSON")
loom pages conformance my-page     # check one page's OKF §11 conformance (-bundle nanite by default)
loom bundles conformance nanite    # check every page in a bundle's OKF §11 conformance
loom export -bundle nanite         # write OKF-style markdown export
loom directives parse < notes.md   # preview directives from text
```

`loom compile`'s input (stdin or `-input`) may be plain text, which becomes
the page body verbatim, or a JSON `compiler.Request` object
(`title`/`slug`/`type`/`summary`/`body`/`source`/`template`/`generation_mode`).
The same request shape is accepted by the `/api/compile-jobs` HTTP endpoint,
the `loom_compile_request` MCP tool, and directive-driven compiles.

Compile jobs default to Loom's deterministic, offline template compiler. Set
`"generation_mode": "llm"` in the request to instead author the page body
with an LLM: export `ANTHROPIC_API_KEY` and, optionally, set `llm.model` in
`config.yaml` (see `config.example.yaml`; defaults to `claude-sonnet-4-6`
when left blank). Without an API key set, `generation_mode=llm` jobs fail
with a clear error — the deterministic path never needs one. Compile-job
responses also include a `confidence`/`confidence_tier` and, when
overwriting an existing page, a `diff` summary, so a caller can tell a safe
write from one that needs review.

Set `LOOM_OTEL_ENABLED=1` to enable OpenTelemetry traces and
`LOOM_OTEL_METRICS_ENABLED=1` to enable traces plus Loom HTTP/MCP metrics.
The equivalent CLI flags are `-otel`, `-otel-metrics`, and `-otel-endpoint`.

## Develop

Two processes during development:

```sh
make run      # Go server on :8080 (serves /api and the last UI build)
make ui-dev   # Vite dev server with hot reload — proxies /api to :8080
```

Open the Vite dev server URL — the app is served at the root path.

## Layout

```
cmd/loom/   Go entrypoint — HTTP server + /api
internal/webui/             //go:embed all:dist + the go-webui handler
frontend/                   Vite + React frontend (the Sysop UI)
  src/App.tsx               app shell — nav rail + page header
  src/pages/                one page per screen
  src/api/                  same-origin API client + typed context
```

The frontend builds into `internal/webui/dist/`, which the Go binary
embeds — so a single binary serves both the API and the UI.

## Adding a page

A page is generic kit chrome plus app-specific content. Add a component
under `frontend/src/pages/`, then wire it into `frontend/src/App.tsx`
(extend the `nav` array and the active-route switch). Add API endpoints
to `frontend/src/api/client.ts`. See the
[`@hollis-labs/sysop-ui` README](https://github.com/hollis-labs/sysop-ui)
for the `PageHeader` / `DataTable` / `SummaryCards` composition pattern.

## Dependencies

- **`@hollis-labs/sysop-ui`** (`v0.9.0`) — the React
  kit + canonical theme. Consumed as a git dependency, pinned to a release
  tag. For local kit development, link a working copy:
  `npm install file:<path-to>/libs/sysop-ui` from `frontend/`.
- **`github.com/hollis-labs/go-webui`** (`v0.1.0`) —
  the SPA-serving harness.

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — data model, compile
  pipeline, directives, the HTTP/CLI/MCP surfaces, and the current status of
  the cross-repo pilot.

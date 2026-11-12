# Loom

Loom compiles supplied material into stored, OKF-shaped wiki pages — either by
deterministic template substitution or a real Anthropic call — and exposes the
same SQLite-backed service layer over HTTP, CLI and MCP. It does not decide what
to compile, when, or with what title and provenance; that is the caller's job
(a router, an agent, or a person at the CLI). It is not a
capture inbox, an agent runtime, or a recall store.

## Start Here

- `docs/architecture.md` is the current-state reference: schema, compile
  pipeline, all three surfaces, and an honest status section on the cross-repo
  pilot.
- `cmd/loom/` is the single entrypoint: `substrate.yaml` is the Substrate
  manifest and `substrate_wiring.go` is generated from it (`make generate`);
  `main.go` boots the modules, `config.go` points them at the data directory,
  `serve.go` builds the handler chain, `cli.go` holds the local commands.
- `internal/service/` is the one service layer HTTP, CLI and MCP all call;
  `confidence.go` owns the confidence/diff signal callers use to decide write
  versus stage-for-review.
- `internal/compiler/` owns deterministic-vs-LLM generation and frontmatter
  rendering.
- `internal/storage/migrations/sqlite/00001_init.sql` is the schema (one goose
  migration, edited in place until the first release).

## Commands

CI runs these (see `.github/workflows/ci.yml`):

```bash
go vet ./...
go run github.com/hollis-labs/substrate/cmd/substrate pins check --app .
go test -race ./...
golangci-lint run          # v2.11.4
go run github.com/hollis-labs/substrate/cmd/substrate scan-public --tree
```

After the frontend steps in `frontend/` (`npm ci`, `npm run typecheck`, `npm run lint`,
`npm run build`, first, because `go build` embeds whatever sits in
`internal/webui/dist/`), and `make build`, CI runs the browser suite
(`npm run test:e2e` in `frontend/`, bundled Chromium via `PLAYWRIGHT_CHANNEL=chromium`,
`LOOM_E2E_PORT`), which starts the real binary on a throwaway data directory. Local runs
use a free port: set `httphost.addr` in the file named by `SUBSTRATE_CONFIG`.

## Boundaries

`internal/webui/dist/.gitkeep` is tracked on purpose. Everything else in that
directory is gitignored, and `//go:embed all:dist` in `internal/webui/embed.go`
fails to compile against an empty directory — which broke CI from the first push
until `0d8981e`. `make clean` preserves the placeholder for the same reason.

The MCP result cache is one shared SQLite table across processes, not
per-process memory. Its owner is the verified principal (`callerID(ctx)` in
`internal/mcp/cache.go`), never a tool argument, and it must never cache under
the `"default"` sentinel (no verified principal) — a shared bucket would let
one caller read another's results. `TestCacheListResult_NoCacheForDefaultCallerID`
and `TestMCPPageSearchCacheDeepDive` guard this.

The CLI resolves project-local `.loom/` paths by default (`resolveLayout` in
`cmd/loom/cli.go`), so which database you get depends on the working directory.
Anything spawning `loom` from another directory must pass `-db`.

Auth is the Substrate guard in `enforce` mode and is not configurable away:
every `/api/` route needs a scope in `api.Routes()` (a route without a line there
is unreachable, not open), `/mcp` needs `loom.mcp`, and `internal/hostguard`
refuses Host names that are not loopback or in `loom.allowed_hosts`. The
listener defaults to `127.0.0.1:8080`; a wildcard address is for a deployment
that already has its owner (created offline with `loom identity setup`). New settings go in `internal/settings` (an
unknown key must stop startup), never in `os.Getenv`; the only environment
inputs are `SUBSTRATE_CONFIG` and the XDG/HOME variables the path layout reads.
Pins are edited only in Substrate's `bom/bom.yaml`; `pins check` must pass.

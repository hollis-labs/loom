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
- `cmd/loom/main.go` is the single entrypoint — subcommand dispatch, path
  layout, OTel wiring.
- `internal/service/` is the one service layer HTTP, CLI and MCP all call;
  `confidence.go` owns the confidence/diff signal callers use to decide write
  versus stage-for-review.
- `internal/compiler/` owns deterministic-vs-LLM generation and frontmatter
  rendering.
- `internal/storage/migrations/*.sql` is the schema, in order.

## Commands

CI runs exactly these, and nothing else:

```bash
go vet ./...
go build ./...
go test ./...
```

`make ui-build` runs `npm install` and rewrites the frontend build. CI never
builds the frontend, and `go build` embeds whatever already sits in
`internal/webui/dist/` — so do not run it to satisfy a check.

## Boundaries

`internal/webui/dist/.gitkeep` is tracked on purpose. Everything else in that
directory is gitignored, and `//go:embed all:dist` in `internal/webui/embed.go`
fails to compile against an empty directory — which broke CI from the first push
until `0d8981e`. `make clean` preserves the placeholder for the same reason.

The MCP result cache is one shared SQLite table across processes, not
per-process memory, so it must never cache under the `"default"` caller_id
sentinel — a shared bucket would let one caller read another's results.
`TestCacheListResult_NoCacheForDefaultCallerID` guards this.

The CLI resolves project-local `.loom/` paths by default (`resolveLayout` in
`cmd/loom/main.go`), so which database you get depends on the working directory.
Anything spawning `loom` from another directory must pass `-db`.

The HTTP listener defaults to `127.0.0.1:8080` on purpose: tokenless loopback
use can trigger paid LLM calls. Do not change the default to a wildcard address;
deployments that need one pass `-addr` explicitly (the Dockerfile does), and
must provide `LOOM_API_TOKEN` or `-token`.

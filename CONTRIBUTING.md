# Contributing

How a change gets from your clone into `main`. This is deliberately short: most
of what you need is written somewhere closer to the thing it describes.

## Before your first change

`README.md` has the prerequisites and build steps. `docs/architecture.md` is
the current-state reference for the schema, compile pipeline and all three
surfaces (HTTP, CLI, MCP). `AGENTS.md` is the fastest orientation to the
layout and the boundaries that are not obvious from reading the code.

## The sequence

1. **Branch.** `<type>/<short-slug>`, where the type matches the change —
   `feat`, `fix`, `docs`, `chore`. Nothing enforces this; it is what the
   history does.
2. **Change one thing.** A branch carrying two unrelated changes costs the
   reviewer the ability to accept one and question the other.
3. **Run the checks** CI runs:
   ```
   go vet ./... && go build ./... && go test ./...
   ```
   `make lint` adds staticcheck, errcheck and govulncheck when installed.
4. **Push and open a pull request.** A maintainer will review it.

Commit subjects follow the conventional-commit shape — a type, an optional
scope, a colon, then the summary.

## What a pull request should carry

The reviewer was not there when you made the decisions. State what the change
does, what it deliberately leaves alone, and the evidence that it works — the
commands you ran and what came back, not a claim that it passes.

Add a line to `CHANGELOG.md` under `[Unreleased]`.

## The one that cannot be undone

**Migrations and stored pages.** The schema is `internal/storage/migrations/*.sql`,
applied in order to a database users keep. Never edit or renumber a migration
that has shipped; add a new one. Compiled pages and their provenance are
durable, so a change to compile or frontmatter output should say what happens
to pages already stored.

## Things that surprise people

- **`internal/webui/dist/.gitkeep` is tracked on purpose.** `//go:embed all:dist`
  fails to compile on an empty directory. Do not delete it; `make clean`
  preserves it.
- **CI never builds the frontend.** `go build` embeds whatever is already in
  `internal/webui/dist/`; `make ui-build` runs `npm install` and rewrites it.
- **The MCP result cache is shared across processes** and must never cache under
  the `"default"` caller ID, or one caller could read another's results.
- **Which database you get depends on the working directory.** The CLI resolves
  a project-local `.loom/` by default; pass `-db` to be explicit.
- **The listener defaults to loopback** because the API is unauthenticated.

## What this does not cover

- **Which change is worth making.** Open an issue to discuss larger features
  before building them.
- **Deciding what to compile.** Loom compiles what it is given; routing and
  judgment live in the caller.
- **Releases.** Maintainers cut those.

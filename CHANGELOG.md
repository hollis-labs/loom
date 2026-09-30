# Changelog

All notable changes to Loom are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Loom is pre-release and has no tagged versions yet; the binary reports `0.1.0`. Entries are backfilled from `git log` and are good-faith, not exhaustive.

## [Unreleased]

### Added

- Open-source project documents: `CHANGELOG.md`, `CONTRIBUTING.md`,
  `SECURITY.md`, `TRADEMARK.md`.

### Changed

- The HTTP listener now defaults to loopback (`127.0.0.1:8080`) because the API
  has no authentication and can trigger paid LLM calls. The Dockerfile passes
  its address explicitly.
- The frontend lockfile resolves `sysop-ui` over HTTPS so a clone without
  GitHub SSH keys can `npm ci`.
- Adopted the `go-mcp` v0.5.0 portfolio standard for the MCP server.
- README rewritten as a pre-release identity and stack-fit document.

### Removed

- `CLAUDE.md` (agent guidance lives in `AGENTS.md`).

## Pre-release history (August–September 2026)

### Added

- Wiki data model with OKF core, lifecycle, trust and provenance fields as real
  columns.
- LLM generation path (Anthropic) with pre-send redaction of secret-shaped
  content, and OKF frontmatter rendering.
- Compile API confidence score and diff summary, so callers can decide between
  writing and staging for review.
- MCP result caching (shared across processes, never under the default
  caller), OKF lint / conformance checks, directive handlers
  (`::draft`, `::log-adr`, `::reminder`, `::extract`), and a regenerable
  export renderer.
- OpenTelemetry tracing, opt-in via `LOOM_OTEL_ENABLED`.
- Embedded Sysop UI frontend, HTTP API, CLI, and MCP server over one shared
  service layer.

### Fixed

- CI: a tracked `internal/webui/dist/.gitkeep` placeholder so `go:embed`
  compiles before the frontend is built.
- Nil-interface panic in OpenTelemetry wiring.

# Security policy

## Supported versions

Loom is pre-release software with no tagged versions. Security fixes are made
on `main`. There are no backports.

## Report a vulnerability

Do not include an exploit, API key, database, exported wiki, or other sensitive
material in a public issue.

Use GitHub's private vulnerability-reporting flow when the repository's
Security tab offers it. If it is unavailable, contact a repository maintainer
privately through a contact channel published on the Hollis Labs organization
or maintainer profile. Include:

- the affected commit and operating system
- the surface involved (HTTP, `/mcp`, stdio MCP, CLI, UI) and the listen address
- reproduction steps and the security impact
- whether credentials or stored content may have been exposed
- a safe way to contact you about coordination

Maintainers will acknowledge a private report, investigate it, and coordinate
disclosure; response times are best effort.

## Deployment boundary

`loom serve` hosts the HTTP API, the embedded Sysop UI and a `/mcp` endpoint on
one listener. **None of it is authenticated.** Anyone who can reach the
listener can read, create and delete pages and can trigger compile jobs that
call a paid LLM using your `ANTHROPIC_API_KEY`.

- The listener defaults to `127.0.0.1:8080`. Only widen it (`-addr :8080`, as
  the Dockerfile does) on a trusted network or behind an authenticating reverse
  proxy that also provides TLS.
- Loom provides no TLS and no rate limiting.
- `loom mcp` (stdio) trusts whatever process launched it.
- The MCP result cache keys on caller ID and never caches under the `"default"`
  caller; report any path that lets one caller read another's cached results.

## Data at rest

Pages, provenance, compile jobs and the MCP cache live in SQLite under the
project-local `.loom/` directory (or the path given with `-db`). The export
directory holds regenerable Markdown copies of pages. Loom has no at-rest
encryption; protect these paths with filesystem permissions and treat exports
like the database.

## External data processors

The deterministic compile path makes no network calls. With
`generation_mode: "llm"`, Loom sends the supplied title, context and source
body to Anthropic. A baseline set of secret-shaped patterns (API keys, AWS
keys, bearer tokens, GitHub tokens, JWTs, PEM blocks) plus any configured
`redact_patterns` are redacted before sending; this is best effort, not a
guarantee. Keep `ANTHROPIC_API_KEY` in the process environment, never in
committed config. Optional OpenTelemetry export is off unless
`LOOM_OTEL_ENABLED` is set.

## Current security limitations

- no authentication or authorization on the HTTP, UI or `/mcp` surfaces
- no built-in TLS
- no at-rest encryption
- pre-release contracts and schema

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
one listener, behind Substrate's auth guard in `enforce` mode. Every `/api/`
request and `/mcp` needs a verified credential: a session cookie from sign-in
(the owner is created offline with `loom identity setup`), or an API key / minted token, holding the scope the route
declares (`loom.read`, `loom.write`, `loom.mcp`). Only `/healthz` and the static
UI are public. Cookie-authenticated writes need an `X-Requested-With` header and
a same-origin `Origin`. The API serves only loopback `Host` names plus
`loom.allowed_hosts` (DNS rebinding); forwarding headers are dropped unless a
listed host arrives from a loopback peer with exactly one
`X-Forwarded-Proto: https`.

- Behind a reverse proxy, run a local TLS proxy on the same host, list the public
  name in `loom.allowed_hosts` and set `identity.trusted_proxy`.
- Compile jobs may call a paid LLM: keep the Anthropic key behind a secret
  reference (`loom.llm_api_key_ref`), never in config, and give keys the scopes
  they need.
- Loom provides no TLS and no rate limiting.
- First-run setup over HTTP is refused (`403 setup_offline`); the owner is created on the host with
  `loom identity setup` before the service starts. Do not set `identity.setup: http`.
- `loom mcp` (stdio) runs under local filesystem trust: the verified principal `local:operator` with exactly
  `loom.mcp`, `loom.read` and `loom.write`, for any process run by a user who can open the data file (on a gateway
  host, any agent the gateway lets reach the child). Set `mcp.stdio_token_file` to require a key instead.
  The local commands (`loom compile`, `pages`, ...) open the database file and
  trust whoever can open it.
- MCP tools are authorized per tool: read tools need `loom.read`, the rest
  `loom.write`, on top of `loom.mcp`.
- The MCP result cache keys on the verified principal and never caches for a
  call without one; report any path that lets one caller read another's cached
  results.

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
guarantee. Set the key as a secret reference, `loom.llm_api_key_ref`
(`keychain://`, `file://`, `op://`, or `env://ANTHROPIC_API_KEY`), never the key
itself in committed config. Optional OpenTelemetry export is off unless
`loom.otel_enabled` is set.

## Current security limitations

- one shared bearer token; no per-user authorization
- no built-in TLS
- no at-rest encryption
- pre-release contracts and schema

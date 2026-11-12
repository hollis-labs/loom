# Cutover plan: running Loom moves to the Substrate build

Status: **draft for the owner's approval. Nothing here has been run.** Merging the port (PR #13, merge commit
`18568a0`) changed no running system. This plan is the separate step that does.

**Pre-merge `main`: `be38c7e`** ("Docs: state the release posture for tokenless loopback"). That is the last commit
of the old version. A fix to the running version is cut from `be38c7e`, not from `main`, until the cutover is done.

Steps marked **PENDING** depend on the offline identity bootstrap (a Substrate identity command that creates the
first owner and a key from the command line, no server) and on local stdio trust. Neither exists yet; those steps are
written against the brief and must be re-checked when the framework change lands.

## What runs today (from the owner side; the live instance was not opened or copied)

| Piece | Where | How it runs |
|---|---|---|
| The served instance | the server host | a user service unit, `loom serve --no-project --addr 127.0.0.1:8092`, no token in the unit; an environment file exists beside the service descriptor under the user's config directory (not opened) |
| Its data | the server host, `~/.local/share/loom/workspaces/default/main.db` (WAL mode, about 156 KB; the online-backup copy held 1 bundle and 1 template, all else empty) | read by the service and by the child below |
| Tether gateway's child | the server host | one `loom mcp --no-project -db ~/.local/share/loom/workspaces/default/main.db`, parent the Tether gateway, serving the `loom_*` tools to agents |
| The owner's laptop | the laptop | three `loom mcp --no-project -db <loom checkout>/.loom/data/workspaces/default/main.db` processes from local agent sessions, binary `~/go/bin/loom`, and a `~/.local/share/loom` there |
| `.loom` directories on the server host | none found | |

Not known: who calls `127.0.0.1:8092`. Ask before cutover (step 1.4).

## Principles

- The old files are never written. `loom migrate-data` opens them read-only, builds a new `loom.db` beside them, and
  refuses to overwrite one. **Rollback is therefore: stop the new build, start the old build on the old files.**
- Migrate a stopped service or an online-backup copy, never beside a writer. The command also refuses if the old file
  changed while it ran.
- One machine, one kind of client at a time, with the check after each. Nothing is switched for a client until its
  new credential has been tried.

## 0. Before the window (anyone, no downtime)

1. Build and publish the release from `main` (CI is green on the merge). Record the commit.
2. Copy the old binary aside on both machines (`loom` of the running version) so rollback is a file swap.
3. Rehearse on a fresh online-backup copy of the live file, in a scratch directory with `HOME` and every XDG variable
   pointed there (the rehearsal procedure and the recorded results are in `docs/conversion-notes.md`). Counts must
   match, `integrity_check` ok, `foreign_key_check` clean, old file checksum unchanged.
4. Inventory the callers of `127.0.0.1:8092` (the owner of the service knows who holds the old token or relies on the
   old CORS origins list). Decide per caller: a key with `loom.read` and `loom.write`, or retire.
5. Inventory every `.loom` directory on the laptop: `find <projects> -name main.db -path '*.loom*'`.

## 1. The served instance (server host)

Downtime: the service is stopped for the migration and restart, expected **under a minute** (the migration took
under 20 ms on the copy; the rest is stop, swap, start and the first-run steps).

1. Write the new service config file (named by `SUBSTRATE_CONFIG`): `httphost.addr: "127.0.0.1:8092"`, `loom.llm_*`
   and `loom.otel_*` if the old environment had them (see the old-to-new table), `loom.allowed_hosts` if it sits
   behind a local TLS proxy. Hand the descriptor change (no `--addr`, no `ANTHROPIC_API_KEY` or `LOOM_OTEL_*`, add
   `SUBSTRATE_CONFIG`) to the owner of the service manager.
2. **Stop the service.** The Tether child (section 2) is also attached to the same file: stop the gateway's loom
   server first, or the migration refuses ("changed during the migration").
3. Back up: copy `~/.local/share/loom/workspaces/default/` (online backup form, `main.db` plus `-wal`/`-shm` are not
   needed if the service is stopped cleanly) to a dated directory.
4. Install the new binary. Run `loom migrate-data --all` (or `--from <main.db> --to <loom.db>`). Expect a JSON report
   per database with counts, `integrity_check: ok`, `foreign_key_check: ok`, `old_file: untouched`.
5. Start the service on the new build. Check `/healthz` (database ok).
6. **PENDING (offline identity bootstrap):** create the first owner and the first keys with the bootstrap command.
   Until it exists the only way is first-run setup over HTTP, `POST /api/setup`, from the host itself while the
   listener is loopback-only; do not widen the listener before that is done.
7. Verify: sign in, one write (compile a page), one read, an unknown `Host` refused (403), `/mcp` with a key
   listing 32 tools.

**Rollback:** stop the new service, put the old binary and the old descriptor back, start. The old `main.db` was
never written; `loom.db` can stay or go.

## 2. The Tether gateway's child (server host)

Today: `loom mcp --no-project -db …/main.db`. After: the same command against the migrated file with a key.

1. Decision pending with the framework: whether a gateway-launched child counts as local (filesystem trust). If yes,
   **PENDING**: no key file; the launch config changes `-db` to `…/workspaces/default/loom.db` only. If no:
   create a key (scopes `loom.mcp loom.read loom.write`), write it to a file readable only by the gateway's user, add
   `mcp.stdio_token_file` to the `SUBSTRATE_CONFIG` file the child is launched with.
2. Change the launch config (Tether side, routed through the driver), restart the child.
3. Verify: the gateway lists the 32 tool names unchanged, one `loom_status` call, one write and one read as an agent.
4. Note: with no verified principal (local trust, once it exists) lists are returned truncated with no deep-dive
   pointer; see "What a client sees without a verified principal" in `docs/conversion-notes.md`.

**Rollback:** point the child back at the old binary and `main.db`.

## 3. The laptop's project-local `.loom` directories

Each checkout is migrated once, from inside it, with the sessions that use it closed.

For each directory from the inventory (step 0.5):
1. Close the agent sessions using that checkout (their `loom mcp` child exits).
2. `cd <checkout> && loom migrate-data` (or `--from .loom/data/workspaces/default/main.db --to …/loom.db`). Check the
   report.
3. **PENDING:** a project-local database used only over stdio should not need an owner, keys or a signing key once
   local trust lands. Until then each needs a key file: run `loom serve` against that file once on a loopback port
   with a temporary `SUBSTRATE_CONFIG` setting the port, `POST /api/setup`, create a key, stop, and point
   `mcp.stdio_token_file` at it. The bootstrap command replaces this.
4. Update the session's MCP config: new `loom` binary path, `-db` to `loom.db`, `mcp.stdio_token_file` (until local
   trust).
5. Verify in that checkout: `loom status`, then an agent session lists tools.

An unmigrated checkout is not harmed by the new binary: `loom mcp` stops at start with the message naming
`loom migrate-data` and changes nothing. **Rollback** per checkout: use the old binary with the old `main.db`.

## 4. After

- Remove nothing for at least a release: old `main.db` files and the old binary are the rollback.
- Update the Cerberus descriptor copy in the repo (`loom.cerberus.yaml`) to what was deployed.
- Rotate nothing now; key rotation for the signing key is available (`/api/identity/signing-key/rotate`) when wanted.
- Record the actual downtime and the report output in the task.

## Expected downtime

| Surface | Downtime |
|---|---|
| The served instance | under a minute |
| The Tether child | one restart of the child |
| Each laptop checkout | the time its sessions are closed, a few minutes |

## Open items

- Who calls the service on the loopback port (step 0.4).
- Local stdio trust and the offline identity bootstrap (framework, pending): steps 1.6, 2.1, 3.3.
- Whether a gateway-launched child counts as local (decision with the Framework Agent).
- The deploy mechanism for the server host (an installer or a manual release swap) is the service owner's.

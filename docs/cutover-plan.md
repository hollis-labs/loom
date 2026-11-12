# Cutover plan: running Loom moves to the Substrate build

Status: **draft for the owner's approval. Nothing here has been run.** Merging the port (PR #13, merge commit
`18568a0`) changed no running system. This plan is the separate step that does.

**Pre-merge `main`: `be38c7e`** ("Docs: state the release posture for tokenless loopback"). That is the last commit
of the old version. A fix to the running version is cut from `be38c7e`, not from `main`, until the cutover is done.

Related drafts: PR #15 (the two-processes test behind section 2) and PR #16 (`deploy.yaml`, section 0 and the
rollbacks).

**The offline identity bootstrap now exists** (Substrate `d453ac8`, adopted): `loom identity setup` and
`loom identity key create` run with no server, and `loom mcp` runs under local filesystem trust. The steps below use
them, and the whole plan was rehearsed end to end on a scratch copy (see "Rehearsal at `d453ac8`"). The cutover still
needs the owner's approval, and it does not start without the bootstrap: nothing here opens a setup window.

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
- **No open setup window, ever.** First-run setup over HTTP (`POST /api/setup`) is not used, not even once and not
  on loopback. Loopback is not a boundary on this host: another application's loopback first-run setup was reached
  today by an agent on the same host, which became its owner. The owner and the first keys are created by the offline
  bootstrap **before the service is started**, and the first check after start is that `GET /api/setup` answers
  `{"setup_required":false}`. If it answers true, stop the service at once.
- One machine, one kind of client at a time, with the check after each. Nothing is switched for a client until its
  new credential has been tried.

## 0. Before the window (anyone, no downtime)

1. Publish the release from `main` (CI is green on the merge): a tarball `loom_<version>_linux_amd64.tar.gz` with
   `SHA256SUMS` and `deploy.yaml`, built by `.github/workflows/build.yml` (PR #16: frontend built before the Go build,
   and a check that the published binary serves the UI). Nothing is tagged or released. Record the commit; it is the
   `--commit` of the deploy.
2. Keep the old binary aside, once, on both machines. `be38c7e` has no `deploy.yaml` and no published artifact, so
   `fleet-install deploy loom --commit be38c7e` cannot bring it back. On the first deploy the installer keeps a
   regular file at the target beside the link as `<name>.pre-fleet-install` (as it did for `basalisk` and `mentat` in
   `~/.local/bin`), so expect `loom.pre-fleet-install`; the driver confirms the name with a `--dry-run` before the real
   deploy. Keep a copy of your own as well. The first rollback is a file swap (see section 1). From the second deploy on, rollback is `fleet-install deploy loom --commit <previous>`.
3. Rehearse on a fresh online-backup copy of the live file, in a scratch directory with `HOME` and every XDG variable
   pointed there (the rehearsal procedure and the recorded results are in `docs/conversion-notes.md`). Counts must
   match, `integrity_check` ok, `foreign_key_check` clean, old file checksum unchanged.
4. Inventory the callers of `127.0.0.1:8092` (the owner of the service knows who holds the old token or relies on the
   old CORS origins list). Decide per caller: a key with `loom.read` and `loom.write`, or retire. These keys are
   created by the bootstrap in step 1.5.
5. Inventory every `.loom` directory on the laptop: `find <projects> -name main.db -path '*.loom*'`.
6. Announce to the agents that use the `loom_*` tools that they will be unavailable for the window (see the downtime
   table).

## 1. The served instance (server host)

Downtime: the service is stopped for the migration and restart, expected **under a minute** (the migration took
under 20 ms on the copy; the rest is stop, migrate, bootstrap, deploy and the first checks). The `loom_*` tools are
unavailable to agents for longer, from step 2 until the end of section 2 (downtime table).

1. Write the new config file `~/.config/loom/loom.yaml` (the path the drop-in in `deploy.yaml` names, named by
   `SUBSTRATE_CONFIG`): `httphost.addr: "127.0.0.1:8092"`, `loom.llm_*` and `loom.otel_*` if the old environment had
   them (see the old-to-new table), `loom.allowed_hosts` if it sits behind a local TLS proxy, and
   nothing for the child (local trust, section 2). The unit change (no `--addr`, `SUBSTRATE_CONFIG` set) is the
   drop-in that `fleet-install` writes; nothing is handed to the owner of the service manager by hand.
2. **Stop the Tether gateway's loom child, then the service.**
   - The child is the Tether gateway's, not Loom's and not mine: **the driver stops it, on the Tether side**, through
     the gateway's own configuration for the loom server (remove or disable the entry and reload the gateway; the
     driver names the exact action). I do not touch the gateway.
   - What agents see while it is stopped: the `loom_*` tools fail (or are absent from the gateway's tool list,
     depending on how the entry is taken out); no other tool is affected and no data is lost.
   - Confirm it is gone before going on: no `loom mcp` process is attached to the host's `main.db`
     (`pgrep -af 'loom mcp'`). The migration also refuses if the file changed while it ran.
   - Then stop the service: `systemctl --user stop loom`.
3. Back up: copy `~/.local/share/loom/workspaces/default/` (`main.db` plus `-wal`/`-shm` are not needed once the
   service is stopped cleanly) to a dated directory.
4. Extract the new binary from the release tarball into a scratch directory (it is not installed yet). As the service's
   user, with its `HOME`, run `loom migrate-data --all`. Expect a JSON report per database with counts,
   `integrity_check: ok`, `foreign_key_check: ok`, `old_file: untouched`. This also creates `loom.db`, so neither
   process ever starts on an empty file (see section 2, concurrent start).
5. **Offline identity bootstrap**, with the service still stopped, as the service's user with its `HOME`, the same
   binary and `SUBSTRATE_CONFIG`, and **the same `--no-project`** as the service (without it the commands work on a
   project-local `.loom`):
   - `loom --no-project identity setup --username NAME` (password from the prompt, or the first line of stdin; never an
     argument). It refuses if an owner exists.
   - `loom --no-project identity key create --name CALLER --scope loom.read --scope loom.write --scope loom.mcp --out PATH`
     for each caller from step 0.4 (a new 0600 file, never overwritten; `--stdout` prints it instead). The child needs
     no key (local trust).
   These create the data directory, the signing key and the owner exactly where the service will look.
   `~/.config/loom/loom.yaml` is mode 600 and holds references only, never a key; the key files are separate.
6. Deploy: `fleet-install deploy loom --commit <sha>`. Its preflight backs up `loom.db`, it installs the release,
   flips `~/.local/bin/loom`, writes the drop-in, restarts `loom.service` and checks `/healthz`. The plan does not rely
   on the installer reloading systemd: before the service is started (use the installer's restart-done flow, or check
   right after the restart and stop if it differs), compare `systemctl --user cat loom.service` with the drop-in in
   `deploy.yaml` (the `ExecStart` without `--addr`, `SUBSTRATE_CONFIG`); if it differs, `systemctl --user daemon-reload`
   and restart.
7. **First check, before anything else:** `GET /api/setup` answers `{"setup_required":false}`. If it does not, stop
   the service (`systemctl --user stop loom`) and investigate; the bootstrap did not take.
8. Verify: sign in with the bootstrap's credentials, one write (compile a page), one read, an unknown `Host` refused
   (403), `/mcp` with a key listing the 30 tools.

**Rollback.** First cutover (the previous version has no descriptor): stop the service, remove the drop-in the
installer wrote (`loom.service.d/20-fleet-exec.conf`) and `systemctl --user daemon-reload`, restore
`~/.local/bin/loom.pre-fleet-install` over the symlink `~/.local/bin/loom` (the name is verified by the `--dry-run`
beforehand; the copy from step 0.2 is the fallback), start. The old `main.db` was never written; `loom.db`
can stay or go. Every later deploy: `fleet-install deploy loom --commit <previous>` (or `fleet-install rollback
loom`); a migration that already ran is not reversed by either, the backup from the preflight is the restore.

## 2. The Tether gateway's child (server host)

Today: `loom mcp --no-project -db …/main.db`. After the cutover the service and this child both open the **same**
`loom.db`. PR #15 runs that topology with the real binary (service plus child, sandboxed home) and answers:

- **(a) Two directories, if the child keeps `-db`.** The service (no `-db`) keeps its identity signing key and
  secrets in the data directory (`~/.local/share/loom/`); a child launched with `-db …/workspaces/default/loom.db`
  would look for its own beside that file (`~/.local/share/loom/workspaces/default/`). Same database, two places for
  the signing key. Since the pin the framework **refuses**: a child launched that way against a database that already
  holds users stops with a message naming the missing signing key, and writes none (tested; also in the rehearsal).
- **(b) Credentials are shared, signing keys must be.** A key created offline (or through the service) authenticates on
  the child's stdio (when one is configured) and on the service's `/mcp` alike, because keys and sessions are rows in
  the database; a wrong stdio token is an error, never a fallback to local trust. Only locally minted JWTs depend on
  the signing key.
- **(c) Concurrent start is not safe, so the order is the rule.** Starting both at the same moment on an *empty* file
  failed in 5 of 5 rounds in one process (`database is locked`, a migration that found its tables already created, or
  both rewriting the key's JWKS temp file); on a migrated file with its key it failed in 0 of 3 rounds in the last run
  but did fail once earlier. The file was always left usable and a lone start afterwards came up. This is a framework
  limit (the open-and-migrate lock across processes is not in the pin; `docs/conversion-notes.md`, gap 9). A child that
  starts, or restarts, while the service is running is clean (tested). Hence: step 1.4 creates the schema, step 1.5 the
  key and owner, the service starts, and the child starts only after the service answers.
- **(d) A write through the child while the service holds a write transaction waits up to the 5 s busy timeout, then
  fails `SQLITE_BUSY`.** Tested with the real binary: a 1.5 s hold is waited out and the write succeeds; an 8 s hold
  makes it fail after 5 s. (My earlier note here said it would retry for about 25 s; that was wrong. Single-statement
  writes go to the one write connection directly, not through `Store.WriteTx`, which is what retries.) Loom's own
  write transactions are short, so it takes a lock held for more than five seconds to show; if it ever does, the fix
  is Loom's (route those writes through `WriteTx`).

**The fix, in this section: one config for both.** Launch the child with the same `SUBSTRATE_CONFIG` file as the
service and **no `-db`**. It then resolves the same data directory, the same signing key and the same
`workspaces/default/loom.db`; the tests show it writes no second key and its write is visible through the service.

1. **Local trust, and what it means here.** `loom mcp` runs as the verified principal `local:operator` with exactly
   `loom.mcp loom.read loom.write`, with no key file. The module trusts any process run by a user who can open the data
   file and cannot tell who launched it: **the Tether gateway serves several agents through this one child, so every
   agent the gateway lets reach it can call every loom tool, writes included**, unless the gateway authorizes per
   caller. That is the owner's decision at approval. To keep per-caller keys instead, create one key for the child in
   step 1.5 and name it in `mcp.stdio_token_file` in the shared config (the service never reads it); nothing else changes.
2. Change the launch config (Tether side, routed through the driver): `loom mcp --no-project`, environment
   `SUBSTRATE_CONFIG=~/.config/loom/loom.yaml`, binary `~/.local/bin/loom`, no `-db`. Start it **after** step 1.7 has
   passed (the service first, the child second; this also holds for every later release that carries a migration).
3. Verify: the gateway lists the 30 tool names unchanged, one `loom_status` call, one write and one read as an agent.
4. The local principal is verified, so the result cache works for the child: a large list leaves a
   `wiki_result://` pointer and `loom_fetch_result` returns it (tested). SIGTERM stops the child cleanly (exit 0).

**Rollback:** point the child back at the old binary and `main.db` (and `-db`), through the driver.

## 3. The laptop's project-local `.loom` directories

Each checkout is migrated once, from inside it, with the sessions that use it closed.

For each directory from the inventory (step 0.5):
1. Close the agent sessions using that checkout (their `loom mcp` child exits).
2. `cd <checkout> && loom migrate-data` (or `--from .loom/data/workspaces/default/main.db --to …/loom.db`). Check the
   report.
3. A project-local database used only over stdio needs no owner and no key: `loom mcp` runs under local trust. Nothing
   is bootstrapped, and nothing opens a setup window. (Its signing key is created beside it at the first start.)
4. Update the session's MCP config: new `loom` binary path, `-db` to `loom.db` (explicit `-db` stays right here: it is
   one process per file, no service beside it). No key file.
5. Verify in that checkout: `loom status`, then an agent session lists tools.

The laptop is not under `fleet-install` (its binary is `~/go/bin/loom`); a darwin target in `deploy.yaml` is a later
choice. An unmigrated checkout is not harmed by the new binary: `loom mcp` stops at start with the message naming
`loom migrate-data` and changes nothing. **Rollback** per checkout: use the old binary with the old `main.db`.

## 4. After

- Remove nothing for at least a release: old `main.db` files and the old binary are the rollback.
- Update the Cerberus descriptor copy in the repo (`loom.cerberus.yaml`) to what was deployed.
- Rotate nothing now; key rotation for the signing key is available (`/api/identity/signing-key/rotate`) when wanted.
- Record the actual downtime and the report output in the task.

## Expected downtime

| Surface | Downtime |
|---|---|
| The served instance | under a minute (stop to healthy, estimate from the rehearsed migration time plus restart) |
| The Tether child | one restart of the child |
| **Loom tools unavailable to agents** (`loom_*` fail), sections 1 and 2 together | **about 10 minutes (estimate, not measured)**: from the child's stop (step 1.2) to the verified restart (step 2.3). The service part is the minute above; the rest is the bootstrap, deploy, first checks and the Tether-side launch change, which the driver applies. Preparing that change in advance, to apply the moment step 1.7 passes, brings it nearer 5 minutes. |
| Each laptop checkout | the time its sessions are closed, a few minutes |

## Open items

- Who calls the service on the loopback port (step 0.4).
- Owner's decision: the gateway child under local trust gives every agent the gateway lets through all three scopes
  (section 2, step 1), or per-caller keys via `mcp.stdio_token_file`.
- The build workflow (PR #16) runs on the org build runner and has not run on a merged commit yet; its first run is
  the proof that the tarball and the UI check work there.
- Installer behaviour the driver verifies with a `--dry-run` at cutover (the plan does not run it): the
  `loom.pre-fleet-install` name, and whether it reloads systemd (the plan checks it either way).
- Simultaneous starts of two processes on one file can lose a race in one of them: a framework limit
  (`docs/conversion-notes.md`, gap 9), worked around by the order, not fixed.

## Rehearsal at `d453ac8`

The whole plan, run on a scratch host (HOME and every XDG directory under the sandbox, port 18141, pids recorded) with
the old binary's real `main.db`: config file mode 600; `migrate-data --all` (counts, `integrity_check: ok`,
`foreign_key_check: ok`, `old_file: untouched`, old `main.db` checksum identical before and after); `identity setup`
and `identity key create --out` with the service stopped; start; `GET /api/setup` = `{"setup_over_http":false,
"setup_required":false}` and `POST /api/setup` = 403 `setup_offline`; sign-in, write, read, unknown `Host` 403,
`/mcp` with the key listing 30 tools; the child with the same config and no `-db` and no key (initialize, 30 tools,
`loom_status`, a read, a write, a list); a child with `-db` refused; one signing key in the data directory; SIGTERM of
child and service (both exit 0 after the fixes recorded in the PR); no process left and the port free. The scratch is
not the live host: the live service, its data and the gateway's child were not touched.

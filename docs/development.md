# Hot-deployable development

The core requirement this document describes: **develop, test, deploy, and
iterate on AXOS without repeatedly rebooting or reflashing the router.**
Firmware flashing (`docs/build-environment.md`, `docs/flashing-and-recovery.md`)
is an integration/release step for genuinely low-level changes — kernel,
early boot, driver integration. Everything else — MCP tools, the Core API,
VPN/firewall/routing/DNS logic, diagnostics, monitoring, the web UI — is a
hot-deployable Go binary or static asset, restarted in place.

Target loop:

```
EDIT → BUILD → TEST LOCALLY → DEPLOY TO STAGING → PROMOTE (atomic) →
RESTART ONLY THE CHANGED SERVICE → HEALTH CHECK → KEEP OR ROLLBACK
```

No router reboot anywhere in that loop.

## Status: what's actually verified

Everything on this page has been exercised — the automated test suite
(99 tests as of this writing) plus, for the parts that matter most,
**the real compiled binaries run against each other as separate OS
processes** (not just `go test`): `axosd serve`, `axos-mcp`, and `axosctl`
started independently, talking over real HTTP/loopback sockets, with
`axos-mcp` proven to survive being killed and restarted mid-transaction
without losing an armed rollback (see `internal/mcp/integration_httpclient_test.go`
for the automated version of that proof). What's **not** verified: any of
this running against a real GT-AX6000, or the `asuswrt` backend's
hardware-specific assumptions (nvram keys, interface names, `wl`/`iptables`/
`wg` output parsing — all marked `(verify)` in that package). Capturing a
real router's state (`axosctl capture --backend asuswrt --host <router>`)
is the natural next step once one is reachable, and would resolve most of
those `(verify)` marks at once by giving the code real data to be checked
against.

## Architecture

```
axosd serve                    (the one long-lived process — router state
  ├─ RouterBackend               lives here: an armed rollback timer, the
  ├─ rollback.Engine             audit log file handle, supervised child
  ├─ audit.Logger                processes)
  ├─ supervisor.Supervisor
  └─ Core API (HTTP/JSON)  ◄──────────┐
                                        │
        ┌───────────────────────────────┼───────────────────────┐
        │                                │                       │
   axos-mcp                         axosctl                 (future) web UI
   (stdio MCP,                      (CLI: status/health/      dev server on
    restartable freely)              deploy/rollback/...)      your laptop
```

Why the HTTP indirection instead of `axos-mcp` embedding a `RouterBackend`
directly: `rollback.Engine.Arm` takes a Go closure, which can't cross a
process boundary, and the timer it owns has to live alongside the backend
it restores against. If `axos-mcp` held its own engine, restarting it
mid-transaction (exactly what you want to be free to do during
development) would silently kill the auto-revert safety guarantee. So the
one real engine lives in `axosd`; everything else is a client of it. See
`internal/rollbackctl`'s package doc for the full reasoning.

## The RouterBackend abstraction

Every frontend (`axos-mcp`, `axosctl`, the future web UI) and `axosd`
itself depend only on the `backend.RouterBackend` interface
(`internal/backend/backend.go`) — never on `nvram`/`iptables`/`wl` calls
directly. Four implementations:

| Backend | What it is | Needs a router? |
|---|---|---|
| `mock` | In-memory, fully mutable fake (`internal/backend/mock`) | No |
| `replay` | Serves a captured, sanitized fixture directory (`internal/backend/replay`) | No |
| `asuswrt` | The real thing: nvram/ip/wl/iptables/wg, local or over SSH (`internal/backend/asuswrt`) | Yes |
| `httpclient` | Calls another `axosd`'s Core API (`internal/backend/httpclient`) | Only indirectly, via that axosd |

Select one with `-backend mock|replay|asuswrt` on `axosd serve`/`axosd mcp`,
or `-backend`/`-fixture`/`-host` on `axosctl capture`/`backend-info`
(`internal/backendselect` is the one place that maps flags to a backend, so
`axosd` and `axosctl` can't drift on what a flag means).

**Claude (or any AI/human contributor) can develop and test the large
majority of AXOS functionality against `mock` alone** — every MCP tool, the
Core API's routes, the deploy/rollback mechanics, and the rollback engine's
safety properties are all covered by tests that never touch a shell or a
network interface for real.

### MockBackend

`mock.New()` returns a backend pre-populated with a plausible GT-AX6000
fixture (interfaces including a WAN/LAN role tag, routes, two clients, both
Wi-Fi radios, `dnsmasq`/`httpd` services, three firewall rules, no VPN
configured — matching a stock router). Every field is mutable at runtime via
`internal/backend/mock/state.go`'s setters — `SetClient`, `RemoveClient`,
`SetVPNTunnel`, `SetService`, `SetResources`, `SetNVRAM`, etc. — so a test
(or an interactive session) can simulate a device joining, a VPN peer
handshaking, a service crashing, or a thermal spike without any real
hardware:

```go
mb := mock.New()
mb.SetService(backend.ServiceStatus{Name: "dnsmasq", Running: false}) // simulate a crash
mb.SetResources(backend.Resources{TemperaturesC: map[string]float64{"cpu": 95.0}}) // simulate a thermal warning
```

Run anything against it locally:

```sh
go run ./cmd/axosd serve -backend mock -api-addr 127.0.0.1:9090
```

### Router capture & ReplayBackend

`axosctl capture` queries any backend and writes a **sanitized** fixture set
— the same files `internal/capture` and `internal/backend/replay` use to
round-trip through each other in tests:

```sh
axosctl capture -backend mock -out testdata/mock          # no router needed
axosctl capture -backend asuswrt -host router.lan -out testdata/gt-ax6000  # not yet run — see "Status" above
```

Sanitization (`internal/capture/sanitize.go`) redacts any nvram value whose
key looks secret-shaped (`psk`, `passwd`, `password`, `_key`, `secret`,
`token`, ...) before anything touches disk — `WriteFixtures` refuses to
write a snapshot that hasn't been through `Sanitize()` first, enforced in
code, not just by convention. **Never** commit a capture that hasn't gone
through `axosctl capture` (which always sanitizes) — see
`docs/security.md` "Secrets at rest".

Fixture files, one per subsystem: `system.json`, `nvram.json`,
`interfaces.json`, `routes.json`, `clients.json`, `wifi.json`,
`services.json`, `firewall.json`, `vpn.json`, `capabilities.json` (a derived
summary — counts and any capture errors).

Replay a captured fixture set instead of connecting to a router:

```sh
axosd serve -backend replay -fixture testdata/mock -api-addr 127.0.0.1:9090
```

`ReplayBackend`'s data is immutable (it's one moment's snapshot);
`system.shell_exec` fails clearly rather than faking a shell; `Backup`/
`Restore` work against a small in-memory store so tools exercising them
don't need special-casing for replay mode.

### Live Development Mode (real router)

```sh
axosd serve -backend asuswrt -host router.lan -api-addr 127.0.0.1:9090
```

Runs every backend command over SSH (`internal/backend/asuswrt/ssh.go`,
shelling out to the local `ssh` binary — no new dependency, inherits your
own `~/.ssh/config`/agent/known_hosts) instead of assuming the process is
already running on the router. `BatchMode=yes` is forced by default
(key-only auth, per `docs/security.md`). **Not yet exercised against a real
router from this environment** — the command construction/quoting is unit
tested, but a live SSH session to a GT-AX6000 has not been part of any
automated run here.

## The deploy loop

```sh
axosctl deploy -root /opt/axos                       # on the router, or a local dir for dev-loop testing
axosctl deploy -root /opt/axos -components axosd,axos-mcp -goos linux -goarch arm64  # cross-compile for the router
axosctl deploy -root /opt/axos -skip-tests            # skip the test-suite gate (not recommended)
```

What it does, in order (`cmd/axosctl/deploy.go`):

1. `go test ./...` (skip with `-skip-tests`) — **a failing test blocks the
   deploy entirely; nothing gets staged.**
2. Cross-compile each named `cmd/` package (`-goos`/`-goarch` for targeting
   the router; empty means build for the host, useful for local dev-loop
   testing against `mock`).
3. Compute a `Manifest` (`internal/deploy/manifest.go`) — every file's
   SHA-256, the git commit, which components changed.
4. `Deploy()` (`internal/deploy/deploy.go`): verify the manifest's checksums
   against what's actually in `staging/`, allocate the next release id,
   `os.Rename(staging, releases/NNNNNN)` (atomic — same filesystem), then
   atomically swap the `current` symlink (create-temp-then-rename, so
   `current` never points at nothing or at a half-written release) and
   update `previous` to whatever `current` pointed at before.

Nothing is restarted automatically — `deploy` only asks "does this become
the new `current`", never "who needs to notice". That's deliberate: you
choose which services actually need restarting for a given change.

```sh
axosctl restart axos-mcp          # restart just the one affected service
axosctl health                    # confirm everything (including it) is healthy
axosctl footprint                 # confirm it didn't cost too much RAM either
```

Every `axosd` restart automatically records a RAM footprint snapshot (a
"baseline" on the very first-ever run, "startup" on each one after) — see
`docs/ram-footprint.md`. On a device with 1GB total RAM, "healthy" and
"didn't grow the addon's footprint more than expected" are both worth
checking before calling a deploy done.

If it's unhealthy:

```sh
axosctl rollback -root /opt/axos                 # back to whatever `previous` points at
axosctl rollback -root /opt/axos -version 000041  # or an explicit release id
axosctl history -root /opt/axos                   # see every release, current/previous marked
```

`Rollback` is a single symlink swap — instant, and itself reversible
(rolling back sets `previous` to what `current` just was, so you can undo
the undo).

### `axosctl dev watch`: the actual hot loop

```sh
axosctl dev watch -watch-dir . -service axos-mcp -root /opt/axos
```

On every `*.go` file change under `-watch-dir` (polled — see
`cmd/axosctl/dev.go`'s `fingerprint`, a deliberately dependency-free
max-mtime+file-count check rather than pulling in `fsnotify`): run the full
deploy pipeline above, and if (and only if) it succeeds, restart every
`-service` named and health-check it. A failed test or failed deploy is
printed and the watcher keeps running — it never exits on a bad iteration,
and it never touches anything else in the repo or on the router beyond the
one restart it was told to do.

## Supervised services

`axosd serve` can own a `supervisor.Supervisor` (`internal/supervisor`) for
hot-deployable sibling processes — start/stop/restart, health checks
(HTTP if a `health_url` is configured, else plain liveness), crash-restart
with exponential backoff that gives up after too many failures in a row.
Register services via `-services-config <path to a JSON file>`:

```json
[
  {
    "name": "axos-monitor",
    "command": "/opt/axos/current/bin/axos-monitor",
    "args": ["-api", "http://127.0.0.1:9090"],
    "health_url": "http://127.0.0.1:9191/healthz",
    "restart_backoff_base": "2s",
    "max_consecutive_restarts": 5
  }
]
```

**No such services exist yet** — `axos-monitor` above is illustrative, not
real; monitoring/VPN/routing daemons are Milestone 3+ (`docs/ROADMAP.md`).
An empty or missing `-services-config` is the normal state today, not a
workaround (`internal/svcconfig`'s package doc says so explicitly): the
mechanism is built and tested (`internal/supervisor`, 9 tests;
`internal/api`'s `/v1/supervisor/services/*` routes, tested), waiting for
something real to register.

`axos-mcp` itself is deliberately **not** a candidate for this: it talks
over stdio to whoever invoked it (typically `ssh router axos-mcp`, freshly
per session), so `axosd` spawning it as a supervised child would connect
its stdio to nothing useful. It's started per-session by whatever's
connecting to it, not supervised as a long-running daemon.

## Router-state rollback vs. release rollback — two different things, same word

Easy to conflate, so stated plainly: `axosctl rollback` (above) undoes an
AXOS **software** deployment — a bad `axos-mcp` build, say. `axosctl
transaction begin/confirm/status` (`docs/safety-rollback.md`) undoes a
**router configuration** change — a firewall rule that locked out
management access. Different mechanisms, different failure modes, and
deliberately not unified into one system: a software rollback is "put the
old binary back"; a config rollback is "the AI might not be reachable to
ask for a rollback, so a timer does it automatically." `transaction` is
intentionally thin CLI vocabulary over the *same* `rollback.Engine` that
backs the MCP `rollback.arm`/`rollback.confirm` tools — not a second
implementation.

## Firmware development stays separate

None of the above touches `firmware/`. Firmware changes accumulate as
patches (`firmware/patches/README.md`) applied to a pinned Merlin release,
built by `firmware/build.sh` (wrapped by `scripts/build-firmware.sh`), and
are an explicit, manual integration step — see
`docs/build-environment.md` and `docs/flashing-and-recovery.md`.
`scripts/deploy-router.sh` (routine AXOS deploys to a real router over SSH)
is a different, much more frequent action than flashing firmware, and
should feel like it: no reboot, seconds not minutes, instantly reversible.

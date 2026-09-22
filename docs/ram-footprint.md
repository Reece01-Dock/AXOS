# RAM footprint tracking

The GT-AX6000 has 1GB of RAM total, shared with stock Broadcom/Merlin
userspace and the wireless drivers. Every megabyte AXOS itself adds is a
megabyte not available to the router doing its actual job — so instead of
guessing, `internal/footprint` measures it and tracks it over time.

This is distinct from `system.resources` / `GET /v1/resources`, which
reports the router's whole-system memory (`backend.Resources`). Footprint
tracking answers a narrower question: **how much of that is AXOS's own
processes, and is that number growing as features get added.**

## How it works

- The very first time `axosd serve` ever runs against a given
  `-footprint` log (the file doesn't exist yet / is empty), it
  automatically records a snapshot labelled `"baseline"` — this is the
  "day one install" measurement, captured with no manual step.
- Every subsequent `axosd serve` start records another snapshot, labelled
  `"startup"`. Since the hot-deploy loop (`docs/development.md`) restarts
  axosd after every deploy, this alone builds a growth timeline across
  releases without anyone remembering to measure anything.
- `axosctl footprint snapshot` (or `POST /v1/footprint/snapshot`) forces an
  on-demand snapshot at any time, with an optional `--label`/`--release` to
  tag what was just added (e.g. `--label post-vpn-feature`).

Each snapshot records:

- `process_rss_kb` — the measuring process's actual resident memory (from
  `/proc/self/status`'s `VmRSS`, Linux only). This is the number that
  matters: real physical memory in use, not Go's notion of allocated
  heap.
- `go_heap_alloc_kb`, `go_sys_kb`, `num_goroutine` — Go runtime internals,
  useful for telling a real leak apart from expected growth.
- `system_total_kb` / `system_used_kb` / `system_free_kb` — the router's
  whole-system memory at the same moment (best-effort, from
  `backend.Resources`), for context: "AXOS is using X out of Y free."

Snapshots are appended as newline-delimited JSON to the file named by
`-footprint` (default `./axosd-footprint.jsonl`), the same pattern as the
audit log (`internal/audit`) but without the audit log's owner-only file
permissions — nothing in a footprint snapshot is sensitive.

## Using it

```sh
# Current usage vs. the recorded baseline, with the delta:
axosctl footprint
# or explicitly:
axosctl footprint show

# Every snapshot ever recorded, oldest first, with per-row deltas:
axosctl footprint history

# Force a snapshot right now, e.g. right after adding a feature:
axosctl footprint snapshot --label post-vpn-feature --release 000012
```

Same data over the Core API, for scripting or a future web UI:

```
GET  /v1/footprint          # current live measurement + baseline
GET  /v1/footprint/history  # every recorded snapshot
POST /v1/footprint/snapshot # force one now ({"label": "...", "release_id": "..."})
```

All three respond `501` if `axosd` wasn't started with `-footprint`
configured (it is by default — see `cmd/axosd`'s flags), matching the same
"nil means not configured, never faked" convention `Supervisor` uses.

## What this doesn't do (yet)

- It measures whichever process calls `footprint.Measure` — in practice
  that's `axosd` itself (the only process wired up to record snapshots so
  far). `axos-mcp` and `axosctl` are short-lived/separate processes with
  their own, much smaller and less interesting, footprints.
- It's a measurement tool, not an enforcement one: nothing here fails a
  deploy or blocks a feature for using "too much" RAM. If the numbers show
  concerning growth, that's a signal to investigate — see the "OBSERVE →
  BASELINE TEST → CHANGE ONE THING" methodology in `docs/performance.md`,
  which applies just as well to memory as to throughput.

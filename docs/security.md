# Security

AXOS gives an AI (and any human client) unrestricted root access to a router
that carries all of a household's/network's traffic. That's the explicit
design goal (see the project brief's "MCP / AI Control" section), not an
oversight — but it means the actual security boundary of this whole platform
is **not** "what can the AI do" (the answer is: anything, by design). It is
**who can reach `axosd` at all**, and **what happens to the secrets this
system necessarily handles**. This document is the honest accounting of both,
kept current as the implementation changes — update it in the same PR as any
change to auth, transport, or secrets handling.

## Threat model

**In scope — things this design defends against:**

- An attacker on the LAN or WAN who does *not* already have a valid SSH key
  for the router.
- Accidental self-lockout: a legitimate AI/human change that breaks
  management access, LAN, or WAN connectivity (the rollback engine's job —
  see `docs/safety-rollback.md`).
- Casual disclosure of secrets (Wi-Fi passphrase, admin password, VPN keys)
  via loose file permissions on backups/logs, or via a compromised *other*
  process on the router reading files it has no business reading.
- Firmware supply-chain drift: building from an unpinned/untrusted source.
- A router that fails to boot after a bad flash (recovery mode — see
  `docs/flashing-and-recovery.md`).

**Explicitly out of scope / accepted risk — things this design does *not*
defend against, by the nature of the project:**

- **A malicious or compromised entity that already has valid SSH access to
  the router.** `system.shell_exec` is unrestricted root. Anyone who can talk
  to `axosd` can rewrite the audit log, disarm rollback, exfiltrate every
  secret on the box, and reflash the firmware. There is no sandboxing of the
  "AI administrator" from the router's own OS, because the brief requires
  full root control. **The entire security model therefore collapses to: who
  can open a channel to `axosd`.** Treat SSH key management (below) as the
  single most important control in this whole document.
- **A physically present attacker with console/JTAG access.** Out of scope
  for firmware-level software controls.
- **Broadcom's own proprietary blobs.** We don't audit them; we trust ASUS's
  and Broadcom's own security posture for the driver/acceleration blobs we
  deliberately keep (see `docs/hardware.md`).

## MCP transport & access control

**As of the hot-deploy architecture (`docs/development.md`), `axosd serve`
binds a real TCP listener** for the Core API (`internal/api`) — this
section is updated from an earlier, stdio-only version of this document;
read it as current, not as a "someday" plan.

**The Core API has no authentication of its own — none.** Any request that
reaches the listening port gets full access: the complete nvram dump
(secrets included), `system.shell_exec`, rollback arm/confirm, supervised
service start/stop/restart. There is no token, no mTLS, no user model. This
is a deliberate scope decision for this milestone, not an oversight, but it
means **the entire security perimeter is "can this request reach the
socket at all"** — restated from the pre-API version of this document,
still true, now resting on a different mechanism:

- **The default bind is `127.0.0.1` only** (`cmd/axosd`'s `-api-addr` flag
  defaults to `127.0.0.1:9090`), enforced as the flag default, not merely
  documented. With that default, reaching the API requires either already
  running a process on the router (equivalent to the old stdio-only
  story — SSH access is still the perimeter) or an SSH-forwarded tunnel to
  that port from elsewhere.
- **`axos-mcp`/`axosctl` run on a dev PC, talking to a router over the
  network** (`docs/development.md` "Live Development Mode") — this is a
  real, intended use case, and it must **not** be achieved by widening
  `-api-addr` to `0.0.0.0` or a LAN-facing address. The correct way: SSH
  port-forward the loopback-bound port out —
  `ssh -L 9090:127.0.0.1:9090 router` — and point `axos-mcp`/`axosctl` at
  `http://127.0.0.1:9090` locally. This keeps SSH as the one authentication
  perimeter and adds zero new exposure. Widening the bind address instead
  is a **standing decision to run an unauthenticated full-root API on the
  network** — never do this as a convenience shortcut; if a real need for
  non-loopback binding ever arises, treat it as the "future network
  transport" case below, not as flipping a flag.
- **Key-based SSH auth only.** Disable password auth on the router
  (`nvram set sshd_pass=0` equivalent in Merlin's SSH settings) before AXOS
  is used for anything beyond lab testing.
- Restrict which SSH keys exist on the router to only the AI client(s) and
  operators who are meant to have full root. There is no lesser-privileged
  tier today — anyone with a key is a full router administrator.
- Treat the AI client's SSH private key (wherever it's held — a secrets
  manager, a local keychain) as a **root credential for the router**, full
  stop.

**The `-actor` flag / `X-Axos-Actor` header is a label, not authentication.**
Both `axosd mcp -actor=X` and every `httpclient.Client` request record `X`
in the audit log for attribution — entirely self-asserted by whoever sends
the request, zero access control. Don't mistake it for an auth mechanism
when reading the audit log's `actor` field: it tells you what the caller
*claimed* to be, not what was verified.

**Before the API is ever bound beyond loopback** (a real future network
transport, not the SSH-tunneled loopback case above): it must ship with, at
minimum, token- or mTLS-based auth, must default to binding LAN-only (never
WAN — see "Network exposure defaults" below), and this document's threat
model must be revisited. There is no reason to rush this; SSH-tunneled
loopback already covers the dev-PC-to-router case the project actually
needs today.

## Network exposure defaults

- `axosd` must never bind a listener to the WAN interface. Any future network
  transport binds to the LAN bridge (or localhost) by default, with WAN
  exposure requiring an explicit, separately-audited opt-in.
- The custom AXOS web UI (planned, not built — see `docs/architecture.md`)
  inherits the same rule: LAN-only by default, same as the stock ASUS UI's
  own default posture.

## Secrets at rest

`config.backup` (both the MCP tool and the `asuswrt.Backend.Backup` method
underneath it) captures the full `nvram show` output, which contains, in
**plaintext**: the Wi-Fi passphrase(s), the router admin panel password, and,
once Milestone 3 lands, WireGuard/OpenVPN private keys stored in nvram or
JFFS. This is unavoidable if backups are to be useful for the rollback engine
(`docs/safety-rollback.md`), which needs to actually restore working Wi-Fi
and VPN config, not a redacted approximation of it.

**Current mitigation (implemented):** every backup file, its metadata
sidecar, the backup directory, and the audit log are created **owner-only**
(`0600`/`0700`), and `axosd` re-asserts that mode on every write rather than
trusting it was set correctly once — see `internal/audit/audit.go`'s `Open`
and `internal/backend/asuswrt/asuswrt.go`'s `writeOwnerOnlyFile`, both
covered by tests (`TestOpen_FileIsOwnerOnly`,
`TestOpen_TightensPreExistingLoosePermissions`,
`TestBackup_WritesOwnerOnlyFiles`,
`TestBackup_DirAlreadyExistsWithLoosePermissions`). This is *filesystem*
protection, not encryption — it protects against another unprivileged
process or user on the router, and against casually copying the USB drive
into an environment with different users, but not against anyone with root
on the router (which, per the threat model above, is anyone who can reach
`axosd` at all) or anyone with physical access to the raw USB storage.

**Not yet implemented — tracked here, do not silently skip:**

- **Encryption at rest for backups.** Needs a key-management decision before
  implementation: a key derived from a passphrase set during AXOS
  provisioning (operator must supply it on every `axosd` restart — a real
  usability cost for an unattended router service) vs. a key held in nvram
  itself (protects against casual disk theft, not against anyone who can
  read nvram, which is most of the threat model already) vs. deferring to
  whatever secure-storage primitive the BCM4912 platform offers, if any
  (**verify** whether one exists — unconfirmed). Do not implement this with
  a hardcoded or derived-with-no-secret "key" — that's security theater and
  worse than clearly documenting the gap, which is what this section does.
- **A dedicated secrets store for VPN private keys** (Milestone 3), separate
  from the general nvram-dump backup, so a routine config backup doesn't
  need to carry active VPN key material every time.
- **Restore-time integrity is checksum-verified already** (SHA-256, checked
  in `asuswrt.Backend.Restore` before any `nvram set` is issued — see
  `TestRestore_RefusesCorruptedBackup`), but that's tamper-*detection*, not
  tamper-*prevention* or confidentiality.

## Audit log confidentiality and integrity

`system.shell_exec` — and any tool call generally — is logged with its full
argument set, including the raw command string, in
`internal/audit/audit.go`. This is intentional and required (docs/mcp-api.md
"every mutating call is audited") but means **the audit log itself becomes a
sensitive file** the moment someone runs a command that touches a secret
(e.g. `cat /etc/some-key`, or a future VPN tool passing a key on the command
line). Mitigations in place: owner-only permissions (above) and, longer-term,
prefer dedicated typed tools over `shell_exec` for anything secret-bearing so
the MCP layer can decide what's safe to log — a `shell_exec` invocation is,
by nature, opaque to the audit layer.

**Integrity:** the audit log is currently a plain append-only file with no
tamper-evidence beyond filesystem permissions — a full-root actor can edit or
truncate it. This is consistent with the threat model above (anyone who can
edit it already has unrestricted root) but means the log should be treated as
a *debugging and accountability aid*, not as forensic-grade evidence against
a compromise. If that guarantee is ever needed, it requires either shipping
audit entries off-box in near-real-time (to a system the router itself can't
retroactively edit) or an append-only/signed log format — neither is
implemented; noted here so nobody assumes otherwise.

## Rollback ≠ a security control

Worth stating plainly since it's easy to conflate: the rollback engine
(`docs/safety-rollback.md`) protects against **accidental** self-lockout from
a legitimate change gone wrong. It is not a defense against a malicious
actor, who can simply not call `rollback.arm`, or call `rollback.confirm`
immediately, or disable the mechanism entirely via `shell_exec`. Its value is
entirely in the "trusted administrator makes an honest mistake" case, which
per the threat model is the realistic day-to-day risk this project needs to
manage — not nation-state-grade tamper resistance against a root-privileged
attacker who's already inside the perimeter.

## Firmware & build integrity

- `firmware/setup-sources.sh` pins `asuswrt-merlin.ng` and `am-toolchains` to
  specific refs and records the resolved commit hashes in
  `firmware/.sources-pinned` (gitignored, machine-local) — never build from a
  floating branch for anything that gets flashed.
- Keep a known-good stock ASUS firmware image checksummed and on hand for
  recovery (`docs/flashing-and-recovery.md`'s "Recovery kit") independent of
  anything this repo builds.
- **Not yet designed:** signing custom-built images. Track upstream
  Asuswrt-Merlin security advisories for the release line we fork from
  (3004.388.x) and pull security-relevant patches forward promptly — forking
  means we inherit the responsibility to keep up, not just the code.
- When `system.update` (Milestone 4+, `docs/mcp-api.md`) is implemented, it
  must verify a signature or checksum against a trusted source before
  flashing anything the AI selected — a network-fetched, AI-selected,
  unverified firmware image is an obvious supply-chain hole and must not
  ship without that check.

## Future: web UI security requirements

Captured now so the eventual implementation (`docs/architecture.md`, "Later /
continuous") starts from a checklist instead of a blank page:

- Session cookies: `Secure`, `HttpOnly`, `SameSite=Strict`.
- CSRF tokens on every mutating request (the stock ASUS UI has had CSRF
  issues historically — don't repeat them).
- The web UI talks to `axosd` the same way any other frontend does (shared
  `RouterBackend`/audit/rollback layer — `docs/architecture.md` "One control
  plane") — it must not get a shortcut path that bypasses audit logging or
  danger-tool rollback enforcement.
- LAN-only by default (see "Network exposure defaults" above); if remote
  access is ever wanted, it goes through a VPN tunnel into the LAN, not
  through opening the web UI to the WAN.
- No lesser-privilege role exists yet in the backend (see "MCP transport &
  access control" above) — decide whether the web UI needs a
  viewer/non-admin tier *before* building it, since retrofitting that later
  is much harder than designing for it from the start.

## Checklist (living — update alongside `docs/ROADMAP.md`)

- [x] Backup files, backup metadata, backup directory: owner-only
      permissions, enforced on every write (tested)
- [x] Audit log: owner-only permissions, enforced on open (tested)
- [x] Restore verifies backup integrity via checksum before applying (tested)
- [x] Core API (`axosd serve`) defaults to binding `127.0.0.1` only,
      enforced as the flag default (`docs/development.md`; use an SSH
      tunnel for dev-PC-to-router access, never widen the bind)
- [ ] SSH password auth disabled on the router (verify during Milestone 1/2
      hardware bring-up; router default may have it enabled)
- [ ] Backup encryption at rest (blocked on a key-management decision, above)
- [ ] Dedicated VPN secrets store (Milestone 3)
- [ ] Firmware image signing / verified `system.update` (Milestone 4+)
- [ ] Core API authentication (token/mTLS) — required before the API is
      ever bound beyond loopback; not needed for the SSH-tunneled loopback
      case the project uses today

# axosd

The AXOS core service daemon. See `docs/architecture.md` for the full design;
this is the quick reference for building and running it.

## Status

Milestone 2 (see `docs/ROADMAP.md`): the `RouterBackend` interface, mock
backend, rollback engine, audit log, and MCP server are implemented and
unit-tested (17 tests, all against the mock backend). The `asuswrt` backend
(`internal/backend/asuswrt`) is written against documented Asuswrt-Merlin/HND
conventions but **has not been run against real hardware** — every
hardware-specific assumption in it is marked `(verify)` in a comment. Treat it
as a draft to validate during the Milestone 2 hardware bring-up, not as tested
code.

## Build

```sh
cd axosd
go build ./...                              # host build
go test ./...                               # unit tests (mock backend only)
GOOS=linux GOARCH=arm64 go build -o axosd ./cmd/axosd   # router binary (static)
```

The router binary is a single static aarch64 ELF binary with no runtime
dependencies — copy it to USB storage and run it directly.

## Running

```sh
# On a dev machine, against the mock backend:
./axosd mcp -backend=mock -audit=./axosd-audit.jsonl -actor=cli:dev

# On the router (once hardware-verified):
./axosd mcp -backend=asuswrt -audit=/mnt/usb1/axos/audit.jsonl -actor=mcp:ai
```

`axosd mcp` speaks MCP (JSON-RPC 2.0, one object per line) on stdin/stdout.
Typical AI-client wiring is over SSH:

```sh
ssh router /mnt/usb1/axos/axosd mcp -backend=asuswrt -actor=mcp:ai
```

## Package layout

```
cmd/axosd/                 CLI entrypoint
internal/backend/          RouterBackend interface + shared types
internal/backend/mock/     in-memory fake — used by every unit test
internal/backend/asuswrt/  real implementation (hardware-unverified)
internal/rollback/         arm/confirm/auto-revert engine (unit-tested)
internal/audit/            append-only JSONL audit log (unit-tested)
internal/mcp/               MCP server: JSON-RPC 2.0, tool registry, dispatch
```

## Adding a new MCP tool

1. Add the method to `backend.RouterBackend` (`internal/backend/backend.go`)
   and implement it on both `mock.Backend` and `asuswrt.Backend`.
2. Add a handler in `internal/mcp/tools.go` and register it in
   `internal/mcp/server.go`'s `registerTools`. Mark `dangerous: true` if it's
   a [danger]-class change per `docs/mcp-api.md`.
3. Add a test in `internal/mcp/server_test.go` exercising it through the
   JSON-RPC surface (not just calling the backend directly) so tool-name
   typos and schema issues get caught.
4. Update `docs/mcp-api.md`'s tool table.

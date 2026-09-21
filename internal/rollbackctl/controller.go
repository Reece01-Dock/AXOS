// Package rollbackctl defines the rollback control surface as seen by a
// frontend (MCP, CLI, web UI) — arm/confirm/status — separately from
// internal/rollback's own Engine, and separately from where that Engine
// physically lives.
//
// Why this indirection exists: rollback.Engine.Arm takes a Go closure as
// its restore function, which cannot cross a process boundary. The engine
// (and the timer it owns) must live in the same process as the
// backend.RouterBackend it restores against — in practice, axosd, the
// long-lived core process — never in axos-mcp, which is meant to be
// restarted freely during development. If axos-mcp held its own
// rollback.Engine, restarting it mid-transaction would silently kill the
// timeout-triggered auto-revert, defeating the entire safety guarantee in
// docs/safety-rollback.md.
//
// So: axosd holds the one real Engine, behind Local. Anything else — an
// axos-mcp process, axosctl — talks to it through a Controller
// implementation that forwards over HTTP (internal/backend/httpclient) to
// axosd's API (internal/api), which is itself just a thin wrapper around
// Local. Same interface, same behavior, whichever side of a process
// boundary you're on.
package rollbackctl

import (
	"context"
	"fmt"
	"time"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/rollback"
)

// Controller is what an MCP/CLI/web-UI frontend needs from rollback:
// arm, confirm, and check status. Implemented by Local (in the same
// process as the Engine) and by an HTTP-forwarding client (a different
// process talking to axosd's API).
type Controller interface {
	// Arm snapshots current state and arms automatic rollback: if Confirm
	// isn't called for this id within timeoutSeconds, the snapshot is
	// restored automatically. Returns the transaction id and its deadline.
	Arm(ctx context.Context, timeoutSeconds int, reason string) (id string, deadline time.Time, err error)

	// Confirm disarms the transaction, keeping the change.
	Confirm(ctx context.Context, id string) error

	// Status returns the currently armed transaction, if any.
	Status(ctx context.Context) (rollback.Status, error)
}

// Local implements Controller directly against an in-process
// rollback.Engine and backend.RouterBackend — this is what axosd (and any
// single-process "axosd mcp" invocation) uses. It owns the "snapshot, then
// arm a restore-this-snapshot closure" composition that used to live in
// internal/mcp/tools.go; centralizing it here means axosd's HTTP API
// handler (internal/api) and an in-process MCP server do the exact same
// thing, not two hand-maintained copies of the same logic.
type Local struct {
	Engine  *rollback.Engine
	Backend backend.RouterBackend
}

func (l *Local) Arm(ctx context.Context, timeoutSeconds int, reason string) (string, time.Time, error) {
	if timeoutSeconds <= 0 {
		return "", time.Time{}, fmt.Errorf("timeout_seconds must be positive")
	}

	// Snapshot before arming, not inside the restore closure: we want the
	// state captured at "about to make the risky change", and we want
	// Arm() to fail up front (before any change is applied) if we can't
	// even take a snapshot, rather than discovering that at restore time.
	snapshot, err := l.Backend.Backup(ctx, "pre-rollback: "+reason)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to snapshot before arming rollback: %w", err)
	}

	restore := func(rctx context.Context) error {
		return l.Backend.Restore(rctx, snapshot.ID)
	}

	id, err := l.Engine.Arm(timeoutSeconds, reason, restore)
	if err != nil {
		return "", time.Time{}, err
	}
	return id, l.Engine.Status().Deadline, nil
}

func (l *Local) Confirm(_ context.Context, id string) error {
	return l.Engine.Confirm(id)
}

func (l *Local) Status(_ context.Context) (rollback.Status, error) {
	return l.Engine.Status(), nil
}

var _ Controller = (*Local)(nil)

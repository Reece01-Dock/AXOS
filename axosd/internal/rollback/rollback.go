// Package rollback implements AXOS's arm/confirm/auto-revert safety pattern
// (docs/safety-rollback.md). Exactly one transaction may be armed at a time:
// dangerous changes are meant to happen one at a time, each independently
// verified.
//
// In-memory only for now (Milestone 2 scope). Milestone-3 hardening adds a
// disk-backed watchdog so an armed transaction survives an axosd crash or
// router reboot — see docs/safety-rollback.md "Surviving axosd crashes".
package rollback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrBusy is returned by Arm when a transaction is already pending.
var ErrBusy = errors.New("rollback: a transaction is already armed")

// ErrNotFound is returned by Confirm/Cancel when the id doesn't match the
// currently armed transaction (already confirmed, already expired, or never existed).
var ErrNotFound = errors.New("rollback: no matching armed transaction")

// RestoreFunc restores the pre-change state. It is called with a context that
// is NOT the caller's request context (the caller may well be gone by the
// time this fires), and its error is recorded but otherwise unhandled -- a
// restore is a best-effort last resort.
type RestoreFunc func(ctx context.Context) error

// EventKind identifies what happened to a transaction, for audit logging.
type EventKind string

const (
	EventArmed                EventKind = "armed"
	EventConfirmed            EventKind = "confirmed"
	EventExpiredRestored      EventKind = "expired_restored"
	EventExpiredRestoreFailed EventKind = "expired_restore_failed"
	EventCancelled            EventKind = "cancelled"
)

// Event is emitted for every transaction state change. Callers (typically
// wired to the audit package) subscribe via Engine.OnEvent.
type Event struct {
	ID        string
	Kind      EventKind
	Timestamp time.Time
	Detail    string // e.g. restore error text, or the reason for the change
}

// Status describes the currently armed transaction, if any.
type Status struct {
	Pending  bool
	ID       string
	ArmedAt  time.Time
	Deadline time.Time
	Reason   string
}

// Engine tracks at most one armed transaction and enforces the
// arm -> confirm|expire lifecycle.
type Engine struct {
	mu      sync.Mutex
	pending *transaction
	nextID  uint64
	onEvent func(Event)

	// now is overridable for deterministic tests.
	now func() time.Time
}

type transaction struct {
	id       string
	reason   string
	armedAt  time.Time
	deadline time.Time
	restore  RestoreFunc
	timer    *time.Timer
}

// New returns a ready-to-use Engine.
func New() *Engine {
	return &Engine{now: time.Now}
}

// OnEvent registers a callback invoked for every lifecycle event. Typically
// wired to the audit logger. Must be called before Arm to avoid missing
// events; not safe to change concurrently with Arm/Confirm/Cancel calls.
func (e *Engine) OnEvent(fn func(Event)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEvent = fn
}

func (e *Engine) emit(ev Event) {
	if e.onEvent != nil {
		e.onEvent(ev)
	}
}

// Arm starts a new transaction: if timeoutSeconds elapse without a matching
// Confirm, restore is invoked automatically. Returns the new transaction id.
// Fails with ErrBusy if a transaction is already pending — callers must
// Confirm or wait out the existing one first ("change one thing at a time").
func (e *Engine) Arm(timeoutSeconds int, reason string, restore RestoreFunc) (string, error) {
	if timeoutSeconds <= 0 {
		return "", fmt.Errorf("rollback: timeoutSeconds must be positive, got %d", timeoutSeconds)
	}
	if restore == nil {
		return "", errors.New("rollback: restore function is required")
	}

	e.mu.Lock()
	if e.pending != nil {
		e.mu.Unlock()
		return "", ErrBusy
	}

	e.nextID++
	id := fmt.Sprintf("txn-%d", e.nextID)
	now := e.now()
	txn := &transaction{
		id:       id,
		reason:   reason,
		armedAt:  now,
		deadline: now.Add(time.Duration(timeoutSeconds) * time.Second),
	}
	txn.restore = restore
	e.pending = txn
	txn.timer = time.AfterFunc(time.Duration(timeoutSeconds)*time.Second, func() {
		e.expire(id)
	})
	e.mu.Unlock()

	e.emit(Event{ID: id, Kind: EventArmed, Timestamp: now, Detail: reason})
	return id, nil
}

// Confirm disarms the transaction, keeping the change. Fails with
// ErrNotFound if id doesn't match the currently pending transaction (e.g. it
// already expired).
func (e *Engine) Confirm(id string) error {
	e.mu.Lock()
	if e.pending == nil || e.pending.id != id {
		e.mu.Unlock()
		return ErrNotFound
	}
	txn := e.pending
	e.pending = nil
	e.mu.Unlock()

	txn.timer.Stop()
	e.emit(Event{ID: id, Kind: EventConfirmed, Timestamp: e.now()})
	return nil
}

// Cancel disarms and immediately runs the restore function, without waiting
// for the timeout. Used when a health check fails before the deadline.
func (e *Engine) Cancel(ctx context.Context, id string) error {
	e.mu.Lock()
	if e.pending == nil || e.pending.id != id {
		e.mu.Unlock()
		return ErrNotFound
	}
	txn := e.pending
	e.pending = nil
	e.mu.Unlock()

	txn.timer.Stop()
	err := txn.restore(ctx)
	if err != nil {
		e.emit(Event{ID: id, Kind: EventExpiredRestoreFailed, Timestamp: e.now(), Detail: err.Error()})
		return fmt.Errorf("rollback: restore failed for cancelled transaction %s: %w", id, err)
	}
	e.emit(Event{ID: id, Kind: EventCancelled, Timestamp: e.now()})
	return nil
}

// Status returns the currently pending transaction, if any.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pending == nil {
		return Status{}
	}
	return Status{
		Pending:  true,
		ID:       e.pending.id,
		ArmedAt:  e.pending.armedAt,
		Deadline: e.pending.deadline,
		Reason:   e.pending.reason,
	}
}

func (e *Engine) expire(id string) {
	e.mu.Lock()
	if e.pending == nil || e.pending.id != id {
		// Already confirmed or cancelled — nothing to do. This can race
		// legitimately: Confirm may have stopped the timer just after it fired.
		e.mu.Unlock()
		return
	}
	txn := e.pending
	e.pending = nil
	e.mu.Unlock()

	// Use a fresh background context: the original caller's request context
	// (if any) is almost certainly long gone by the time a timeout fires.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := txn.restore(ctx); err != nil {
		e.emit(Event{ID: id, Kind: EventExpiredRestoreFailed, Timestamp: e.now(), Detail: err.Error()})
		return
	}
	e.emit(Event{ID: id, Kind: EventExpiredRestored, Timestamp: e.now(), Detail: txn.reason})
}

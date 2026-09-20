// Package audit provides an append-only JSONL audit log for every mutating
// action AXOS takes, as required by docs/mcp-api.md and docs/safety-rollback.md.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Entry is one audit log record. Fields are deliberately flat and JSON-tagged
// for easy grepping/parsing on the router.
type Entry struct {
	Timestamp time.Time              `json:"ts"`
	Actor     string                 `json:"actor"`  // e.g. "mcp:ai", "cli:local", "webui:user"
	Action    string                 `json:"action"` // e.g. "wifi.set_channel", "system.shell_exec"
	Args      map[string]interface{} `json:"args,omitempty"`
	Result    string                 `json:"result"` // "ok" | "error" | "reverted"
	Error     string                 `json:"error,omitempty"`
	TxnID     string                 `json:"txn_id,omitempty"` // rollback transaction id, if any
}

// Logger appends Entries to an underlying writer as newline-delimited JSON.
// Safe for concurrent use.
type Logger struct {
	mu sync.Mutex
	w  io.Writer
	// closer is non-nil when Logger owns the underlying file (opened via Open).
	closer io.Closer
	// now is overridable in tests for deterministic timestamps.
	now func() time.Time
}

// New wraps an existing writer (e.g. an already-open file, or a buffer in tests).
func New(w io.Writer) *Logger {
	return &Logger{w: w, now: time.Now}
}

// Open opens (creating if needed) an append-only log file at path and
// returns a Logger writing to it. Call Close when done.
func Open(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	l := New(f)
	l.closer = f
	return l, nil
}

// Close closes the underlying file if Logger owns one (i.e. was created via Open).
func (l *Logger) Close() error {
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}

// Log appends one entry, filling in Timestamp if it's zero.
func (l *Logger) Log(e Entry) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = l.now()
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: marshal entry: %w", err)
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.w.Write(line); err != nil {
		return fmt.Errorf("audit: write entry: %w", err)
	}
	return nil
}

// Success is a convenience for a successful mutating call.
func (l *Logger) Success(actor, action string, args map[string]interface{}, txnID string) error {
	return l.Log(Entry{Actor: actor, Action: action, Args: args, Result: "ok", TxnID: txnID})
}

// Failure is a convenience for a failed call.
func (l *Logger) Failure(actor, action string, args map[string]interface{}, txnID string, err error) error {
	e := Entry{Actor: actor, Action: action, Args: args, Result: "error", TxnID: txnID}
	if err != nil {
		e.Error = err.Error()
	}
	return l.Log(e)
}

// Reverted records that a change was automatically rolled back.
func (l *Logger) Reverted(action, reason, txnID string) error {
	return l.Log(Entry{Actor: "rollback-engine", Action: action, Result: "reverted", Error: reason, TxnID: txnID})
}

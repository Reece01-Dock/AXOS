package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLog_WritesValidJSONL(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)
	l.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

	if err := l.Success("mcp:ai", "wifi.set_channel", map[string]interface{}{"radio": "wl1", "channel": 149}, "txn-1"); err != nil {
		t.Fatalf("Success: %v", err)
	}
	if err := l.Failure("cli:local", "config.restore", nil, "", errors.New("boom")); err != nil {
		t.Fatalf("Failure: %v", err)
	}
	if err := l.Reverted("firewall.rules.set", "management unreachable", "txn-2"); err != nil {
		t.Fatalf("Reverted: %v", err)
	}

	sc := bufio.NewScanner(&buf)
	var entries []Entry
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line not valid JSON: %v (%q)", err, sc.Text())
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	if entries[0].Actor != "mcp:ai" || entries[0].Action != "wifi.set_channel" || entries[0].Result != "ok" || entries[0].TxnID != "txn-1" {
		t.Fatalf("entry[0] = %+v", entries[0])
	}
	if entries[0].Args["radio"] != "wl1" {
		t.Fatalf("entry[0].Args = %+v, missing radio", entries[0].Args)
	}

	if entries[1].Result != "error" || entries[1].Error != "boom" {
		t.Fatalf("entry[1] = %+v", entries[1])
	}

	if entries[2].Result != "reverted" || entries[2].Actor != "rollback-engine" || entries[2].TxnID != "txn-2" {
		t.Fatalf("entry[2] = %+v", entries[2])
	}

	for i, e := range entries {
		if e.Timestamp.IsZero() {
			t.Errorf("entry[%d] has zero timestamp", i)
		}
	}
}

func TestLog_FillsTimestampWhenZero(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)
	fixed := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	l.now = func() time.Time { return fixed }

	if err := l.Log(Entry{Actor: "test", Action: "noop", Result: "ok"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	var e Entry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !e.Timestamp.Equal(fixed) {
		t.Fatalf("Timestamp = %v, want %v", e.Timestamp, fixed)
	}
}

func TestLog_PreservesExplicitTimestamp(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)
	explicit := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := l.Log(Entry{Timestamp: explicit, Actor: "test", Action: "noop", Result: "ok"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	var e Entry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !e.Timestamp.Equal(explicit) {
		t.Fatalf("Timestamp = %v, want %v (should not be overwritten)", e.Timestamp, explicit)
	}
}

// Audit entries can contain command text and output verbatim (see
// system.shell_exec) which may include secrets, so the log file must never
// be group- or world-readable. This guards against that regressing silently.
func TestOpen_FileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	assertOwnerOnly(t, path)
}

func TestOpen_TightensPreExistingLoosePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("seeding pre-existing file: %v", err)
	}

	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	assertOwnerOnly(t, path)
}

func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("%s has mode %o, want 0600 (owner-only — this file can contain secrets)", path, got)
	}
}

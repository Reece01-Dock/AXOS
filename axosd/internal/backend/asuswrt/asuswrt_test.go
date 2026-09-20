package asuswrt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner is a commandRunner test double that never touches a real shell
// or router. It returns a canned response per command name (matched via a
// caller-supplied function) and records every invocation for assertions.
type fakeRunner struct {
	calls   []string // "name arg1 arg2 ..." per call, in order
	respond func(name string, args []string) (stdout, stderr string, exitCode int)
}

func (f *fakeRunner) run(_ context.Context, _ time.Duration, name string, args ...string) (string, string, int, bool, error) {
	f.calls = append(f.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	if f.respond == nil {
		return "", "", 0, false, nil
	}
	stdout, stderr, code := f.respond(name, args)
	return stdout, stderr, code, false, nil
}

func newTestBackend(t *testing.T, r *fakeRunner) *Backend {
	t.Helper()
	b, err := New(WithBackupDir(t.TempDir()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.runner = r
	return b
}

// Secrets test data: nvram show output containing exactly the kind of
// plaintext secret (Wi-Fi passphrase) that makes backup file permissions a
// real security control and not just hygiene.
const fakeNvramShow = "productid=GT-AX6000\nwl1_wpa_psk=SuperSecretWifiPassword123\nhttp_passwd=hunter2\nsize: 3 entries"

func TestBackup_WritesOwnerOnlyFiles(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return fakeNvramShow, "", 0
	}}
	b := newTestBackend(t, r)

	info, err := b.Backup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	assertMode(t, b.BackupDir, 0700)
	assertMode(t, info.Path, 0600)
	assertMode(t, filepath.Join(b.BackupDir, info.ID+".meta.json"), 0600)

	data, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatalf("reading backup file: %v", err)
	}
	if !strings.Contains(string(data), "SuperSecretWifiPassword123") {
		t.Fatal("backup file should contain the nvram dump (sanity check on the fake)")
	}
}

func TestBackup_DirAlreadyExistsWithLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("seeding pre-existing dir: %v", err)
	}

	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return fakeNvramShow, "", 0
	}}
	b, err := New(WithBackupDir(backupDir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.runner = r

	if _, err := b.Backup(context.Background(), "test"); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	assertMode(t, backupDir, 0700)
}

func TestRestore_VerifiesChecksumAndReplaysNvramSet(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		if name == "nvram" && len(args) > 0 && args[0] == "show" {
			return fakeNvramShow, "", 0
		}
		return "", "", 0
	}}
	b := newTestBackend(t, r)

	info, err := b.Backup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	r.calls = nil // reset call log so we only inspect Restore's calls
	if err := b.Restore(context.Background(), info.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	var sawWpaPsk, sawCommit bool
	for _, c := range r.calls {
		if strings.Contains(c, "nvram set wl1_wpa_psk=SuperSecretWifiPassword123") {
			sawWpaPsk = true
		}
		if c == "nvram commit" {
			sawCommit = true
		}
	}
	if !sawWpaPsk {
		t.Errorf("Restore did not replay wl1_wpa_psk via nvram set; calls: %v", r.calls)
	}
	if !sawCommit {
		t.Errorf("Restore did not call nvram commit; calls: %v", r.calls)
	}
}

func TestRestore_RefusesCorruptedBackup(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return fakeNvramShow, "", 0
	}}
	b := newTestBackend(t, r)

	info, err := b.Backup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Tamper with the stored backup after the checksum was recorded.
	if err := os.WriteFile(info.Path, []byte("tampered data"), 0600); err != nil {
		t.Fatalf("tampering with backup file: %v", err)
	}

	r.calls = nil
	err = b.Restore(context.Background(), info.ID)
	if err == nil {
		t.Fatal("Restore of a tampered backup should fail checksum verification")
	}
	if len(r.calls) != 0 {
		t.Fatalf("Restore should not have issued any nvram commands after a checksum failure; calls: %v", r.calls)
	}
}

func TestRestore_UnknownBackupID(t *testing.T) {
	b := newTestBackend(t, &fakeRunner{})
	if err := b.Restore(context.Background(), "does-not-exist"); err == nil {
		t.Fatal("Restore of an unknown backup id should fail")
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s has mode %o, want %o", path, got, want)
	}
}

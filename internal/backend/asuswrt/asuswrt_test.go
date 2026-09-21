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

func TestNVRAMDump_ParsesShowOutput(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return fakeNvramShow, "", 0
	}}
	b := newTestBackend(t, r)

	dump, err := b.NVRAMDump(context.Background())
	if err != nil {
		t.Fatalf("NVRAMDump: %v", err)
	}
	if dump["productid"] != "GT-AX6000" {
		t.Errorf("dump[productid] = %q, want GT-AX6000", dump["productid"])
	}
	if dump["wl1_wpa_psk"] != "SuperSecretWifiPassword123" {
		t.Errorf("dump[wl1_wpa_psk] = %q, want SuperSecretWifiPassword123", dump["wl1_wpa_psk"])
	}
	if _, ok := dump["size: 3 entries"]; ok {
		t.Error("NVRAMDump should not include the trailing summary line as a key")
	}
}

const fakePsOutput = "  PID USER       TIME  COMMAND\n" +
	"    1 admin      0:01 init\n" +
	"  512 nobody     0:03 dnsmasq --conf-file=/etc/dnsmasq.conf\n" +
	"  498 admin      0:10 httpd\n"

func TestServices_DetectsRunningAndStopped(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return fakePsOutput, "", 0
	}}
	b := newTestBackend(t, r)

	svcs, err := b.Services(context.Background())
	if err != nil {
		t.Fatalf("Services: %v", err)
	}

	byName := make(map[string]bool)
	pidByName := make(map[string]int)
	for _, s := range svcs {
		byName[s.Name] = s.Running
		pidByName[s.Name] = s.PID
	}

	if !byName["dnsmasq"] || pidByName["dnsmasq"] != 512 {
		t.Errorf("dnsmasq = running=%v pid=%d, want running=true pid=512", byName["dnsmasq"], pidByName["dnsmasq"])
	}
	if !byName["httpd"] || pidByName["httpd"] != 498 {
		t.Errorf("httpd = running=%v pid=%d, want running=true pid=498", byName["httpd"], pidByName["httpd"])
	}
	if byName["wireguard"] {
		t.Error("wireguard should be reported not running (absent from ps output)")
	}
	if byName["openvpn"] {
		t.Error("openvpn should be reported not running (absent from ps output)")
	}
}

const fakeIptablesFilter = "-P INPUT ACCEPT\n-P FORWARD DROP\n-A INPUT -i eth0 -j DROP\n-A FORWARD -i br0 -o eth0 -j ACCEPT\n"
const fakeIptablesNat = "-P PREROUTING ACCEPT\n-A POSTROUTING -o eth0 -j MASQUERADE\n"

func TestFirewallRules_ParsesFilterAndNatTables(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		for _, a := range args {
			if a == "nat" {
				return fakeIptablesNat, "", 0
			}
		}
		return fakeIptablesFilter, "", 0
	}}
	b := newTestBackend(t, r)

	rules, err := b.FirewallRules(context.Background())
	if err != nil {
		t.Fatalf("FirewallRules: %v", err)
	}

	var sawFilterInput, sawNatPostrouting, sawPolicyLine bool
	for _, rule := range rules {
		if rule.Table == "filter" && rule.Chain == "INPUT" && rule.Rule == "-i eth0 -j DROP" {
			sawFilterInput = true
		}
		if rule.Table == "nat" && rule.Chain == "POSTROUTING" && strings.Contains(rule.Rule, "MASQUERADE") {
			sawNatPostrouting = true
		}
		if strings.HasPrefix(rule.Rule, "-P") {
			sawPolicyLine = true
		}
	}
	if !sawFilterInput {
		t.Errorf("missing filter/INPUT rule; got %+v", rules)
	}
	if !sawNatPostrouting {
		t.Errorf("missing nat/POSTROUTING rule; got %+v", rules)
	}
	if sawPolicyLine {
		t.Error("policy lines (-P ...) should be filtered out, not returned as rules")
	}
}

func TestVPNStatus_NoWgBinary_ReturnsEmptyNotError(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return "", "wg: not found", 127
	}}
	b := newTestBackend(t, r)

	tunnels, err := b.VPNStatus(context.Background())
	if err != nil {
		t.Fatalf("VPNStatus should not error when wg is absent, got: %v", err)
	}
	if len(tunnels) != 0 {
		t.Fatalf("VPNStatus with no wg binary = %v, want empty", tunnels)
	}
}

const fakeWgShowDump = "wg0\tprivkeyredacted\tpubkeyredacted\t51820\toff\n" +
	"wg0\tPEERPUBKEY123\t(none)\t203.0.113.5:51820\t10.8.0.2/32\t1700000000\t12345\t67890\toff\n"

func TestVPNStatus_ParsesWgShowDump(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return fakeWgShowDump, "", 0
	}}
	b := newTestBackend(t, r)

	tunnels, err := b.VPNStatus(context.Background())
	if err != nil {
		t.Fatalf("VPNStatus: %v", err)
	}
	if len(tunnels) != 1 {
		t.Fatalf("VPNStatus = %+v, want exactly one tunnel", tunnels)
	}
	tun := tunnels[0]
	if tun.Name != "wg0" || !tun.Up || tun.Type != "wireguard" {
		t.Errorf("tunnel = %+v, want Name=wg0 Up=true Type=wireguard", tun)
	}
	if len(tun.Peers) != 1 {
		t.Fatalf("tunnel.Peers = %+v, want exactly one peer", tun.Peers)
	}
	peer := tun.Peers[0]
	if peer.PublicKey != "PEERPUBKEY123" {
		t.Errorf("peer.PublicKey = %q, want PEERPUBKEY123", peer.PublicKey)
	}
	if peer.RxBytes != 12345 || peer.TxBytes != 67890 {
		t.Errorf("peer rx/tx = %d/%d, want 12345/67890", peer.RxBytes, peer.TxBytes)
	}
	if peer.LastHandshake.IsZero() {
		t.Error("peer.LastHandshake should be parsed from the dump")
	}

	// The tunnel's own private key must never appear anywhere in the result.
	dumpBytes := []byte(tun.Name + tun.Type)
	for _, p := range tun.Peers {
		dumpBytes = append(dumpBytes, []byte(p.PublicKey+p.Endpoint)...)
	}
	if strings.Contains(string(dumpBytes), "privkeyredacted") {
		t.Error("VPNStatus result must never contain the tunnel's private key")
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

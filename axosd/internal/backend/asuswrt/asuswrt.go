// Package asuswrt implements backend.RouterBackend against a real
// Asuswrt-Merlin router (nvram, /proc, /sys, ip, wl, dnsmasq leases).
//
// STATUS: written against Asuswrt-Merlin/HND conventions and documentation,
// but NOT YET VERIFIED ON REAL GT-AX6000 HARDWARE (see docs/ROADMAP.md
// Milestone 2). Several details — exact nvram key names for this firmware
// branch, wl(8) output format, interface naming — are marked "(verify)"
// below and must be checked against `docs/hardware.md`'s device-facts
// capture before this backend is trusted for anything beyond read-only
// inspection. Treat every method here as a first draft to validate on-device,
// not as ground truth.
package asuswrt

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/reece01-dock/axos/axosd/internal/backend"
)

// Backend talks to a real Asuswrt-Merlin router via nvram/rc/ip/wl and the
// filesystem. All state lives on the router itself; Backend holds only
// configuration (paths), not data.
type Backend struct {
	// BackupDir is where config.backup snapshots are stored. Should be on
	// USB storage, not internal flash (docs/architecture.md "Data placement").
	BackupDir string
	// runner executes commands; overridable in tests to avoid touching a
	// real router. Defaults to execRunner (os/exec).
	runner commandRunner
}

// Option configures a new Backend.
type Option func(*Backend)

// WithBackupDir overrides the default backup directory.
func WithBackupDir(dir string) Option {
	return func(b *Backend) { b.BackupDir = dir }
}

// New returns an asuswrt Backend. It does not verify the router environment
// (nvram/ip/wl availability) at construction time — individual calls fail
// clearly if a required tool is missing.
func New(opts ...Option) (*Backend, error) {
	b := &Backend{
		// (verify) default USB mount label/layout on GT-AX6000; Merlin
		// typically mounts USB storage under /tmp/mnt/<label> or /mnt/<label>.
		BackupDir: "/mnt/usb1/axos/backups",
		runner:    execRunner{},
	}
	for _, opt := range opts {
		opt(b)
	}
	return b, nil
}

// --- command execution -----------------------------------------------------

type commandRunner interface {
	run(ctx context.Context, timeout time.Duration, name string, args ...string) (stdout, stderr string, exitCode int, timedOut bool, err error)
}

type execRunner struct{}

func (execRunner) run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, string, int, bool, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	timedOut := cctx.Err() == context.DeadlineExceeded

	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else if !timedOut {
			return stdout.String(), stderr.String(), -1, timedOut, fmt.Errorf("exec %s: %w", name, err)
		}
	}
	return stdout.String(), stderr.String(), exitCode, timedOut, nil
}

func (b *Backend) run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	stdout, stderr, code, timedOut, err := b.runner.run(ctx, timeout, name, args...)
	if err != nil {
		return "", err
	}
	if timedOut {
		return "", fmt.Errorf("%s %s: timed out", name, strings.Join(args, " "))
	}
	if code != 0 {
		return "", fmt.Errorf("%s %s: exit %d: %s", name, strings.Join(args, " "), code, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// nvramGet reads a single nvram key. Returns "" (no error) if unset, matching
// nvram(8)'s own behavior for missing keys.
func (b *Backend) nvramGet(ctx context.Context, key string) (string, error) {
	out, err := b.run(ctx, 5*time.Second, "nvram", "get", key)
	if err != nil {
		return "", fmt.Errorf("nvram get %s: %w", key, err)
	}
	return strings.TrimSpace(out), nil
}

// --- SystemInfo --------------------------------------------------------

func (b *Backend) Info(ctx context.Context) (backend.SystemInfo, error) {
	var info backend.SystemInfo

	// (verify) nvram keys for this firmware branch; these match documented
	// Asuswrt/Merlin conventions as of the 3004.388.x line.
	productID, _ := b.nvramGet(ctx, "productid")
	buildno, _ := b.nvramGet(ctx, "buildno")
	extendno, _ := b.nvramGet(ctx, "extendno")
	serial, _ := b.nvramGet(ctx, "et0macaddr") // MAC-derived pseudo-serial; (verify) a real serial nvram key if one exists

	info.Model = productID
	info.FirmwareVer = buildno
	info.FirmwareRev = extendno
	info.Serial = serial

	uptime, boot, err := readUptime()
	if err != nil {
		return info, fmt.Errorf("asuswrt: reading uptime: %w", err)
	}
	info.Uptime = uptime
	info.BootTime = boot

	return info, nil
}

func readUptime() (time.Duration, time.Time, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, time.Time{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, time.Time{}, fmt.Errorf("unexpected /proc/uptime format: %q", string(data))
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("parsing /proc/uptime: %w", err)
	}
	d := time.Duration(secs * float64(time.Second))
	return d, time.Now().Add(-d), nil
}

// --- Resources -----------------------------------------------------------

func (b *Backend) Resources(ctx context.Context) (backend.Resources, error) {
	var res backend.Resources
	res.TemperaturesC = make(map[string]float64)

	load, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return res, fmt.Errorf("asuswrt: reading /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(load))
	if len(fields) >= 3 {
		res.CPULoad1, _ = strconv.ParseFloat(fields[0], 64)
		res.CPULoad5, _ = strconv.ParseFloat(fields[1], 64)
		res.CPULoad15, _ = strconv.ParseFloat(fields[2], 64)
	}

	meminfo, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return res, fmt.Errorf("asuswrt: reading /proc/meminfo: %w", err)
	}
	sc := bufio.NewScanner(strings.NewReader(string(meminfo)))
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		val, _ := strconv.ParseUint(parts[1], 10, 64)
		switch key {
		case "MemTotal":
			res.MemTotalKB = val
		case "MemFree":
			res.MemFreeKB = val
		}
	}
	if res.MemTotalKB > 0 {
		res.MemUsedKB = res.MemTotalKB - res.MemFreeKB
	}

	// (verify) thermal zone paths/names on BCM4912 — /sys/class/thermal is
	// the standard Linux interface, but zone->component mapping is
	// device-specific and needs confirming against real hardware.
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, zonePath := range zones {
		raw, err := os.ReadFile(zonePath)
		if err != nil {
			continue
		}
		milliC, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			continue
		}
		zoneDir := filepath.Dir(zonePath)
		name := filepath.Base(zoneDir)
		if typeRaw, err := os.ReadFile(filepath.Join(zoneDir, "type")); err == nil {
			name = strings.TrimSpace(string(typeRaw))
		}
		res.TemperaturesC[name] = float64(milliC) / 1000.0
	}

	return res, nil
}

// --- Interfaces ------------------------------------------------------------

type ipLinkEntry struct {
	IfName    string `json:"ifname"`
	Address   string `json:"address"`
	OperState string `json:"operstate"`
	LinkType  string `json:"link_type"`
}

type ipAddrEntry struct {
	IfName   string `json:"ifname"`
	AddrInfo []struct {
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

func (b *Backend) Interfaces(ctx context.Context) ([]backend.Interface, error) {
	linkOut, err := b.run(ctx, 5*time.Second, "ip", "-j", "link")
	if err != nil {
		return nil, fmt.Errorf("asuswrt: ip -j link: %w", err)
	}
	var links []ipLinkEntry
	if err := json.Unmarshal([]byte(linkOut), &links); err != nil {
		return nil, fmt.Errorf("asuswrt: parsing ip -j link output: %w", err)
	}

	addrOut, err := b.run(ctx, 5*time.Second, "ip", "-j", "addr")
	if err != nil {
		return nil, fmt.Errorf("asuswrt: ip -j addr: %w", err)
	}
	var addrs []ipAddrEntry
	if err := json.Unmarshal([]byte(addrOut), &addrs); err != nil {
		return nil, fmt.Errorf("asuswrt: parsing ip -j addr output: %w", err)
	}
	addrByIface := make(map[string][]string)
	for _, a := range addrs {
		for _, ai := range a.AddrInfo {
			addrByIface[a.IfName] = append(addrByIface[a.IfName], fmt.Sprintf("%s/%d", ai.Local, ai.PrefixLen))
		}
	}

	out := make([]backend.Interface, 0, len(links))
	for _, l := range links {
		iface := backend.Interface{
			Name:      l.IfName,
			Type:      classifyInterface(l.IfName, l.LinkType),
			MAC:       l.Address,
			Addresses: addrByIface[l.IfName],
		}
		if l.OperState == "up" {
			iface.State = backend.IfaceUp
		} else if l.OperState == "down" {
			iface.State = backend.IfaceDown
		} else {
			iface.State = backend.IfaceUnknown
		}

		if speed, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/speed", l.IfName)); err == nil {
			if mbps, err := strconv.Atoi(strings.TrimSpace(string(speed))); err == nil && mbps > 0 {
				if mbps >= 1000 {
					iface.LinkSpeed = fmt.Sprintf("%.1fGbps", float64(mbps)/1000.0)
				} else {
					iface.LinkSpeed = fmt.Sprintf("%dMbps", mbps)
				}
			}
		}

		readCounter := func(stat string) uint64 {
			raw, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/statistics/%s", l.IfName, stat))
			if err != nil {
				return 0
			}
			v, _ := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
			return v
		}
		iface.RxBytes = readCounter("rx_bytes")
		iface.TxBytes = readCounter("tx_bytes")
		iface.RxErrors = readCounter("rx_errors")
		iface.TxErrors = readCounter("tx_errors")
		iface.RxDropped = readCounter("rx_dropped")
		iface.TxDropped = readCounter("tx_dropped")

		out = append(out, iface)
	}
	return out, nil
}

// classifyInterface guesses an interface's role from its name. (verify)
// against actual GT-AX6000 interface naming captured in docs/hardware.md —
// this is a best-effort heuristic, not a source of truth.
func classifyInterface(name, linkType string) string {
	switch {
	case strings.HasPrefix(name, "br"):
		return "bridge"
	case strings.HasPrefix(name, "wl"):
		return "wifi"
	case strings.HasPrefix(name, "tun"), strings.HasPrefix(name, "tap"), strings.HasPrefix(name, "wg"):
		return "vpn"
	case strings.HasPrefix(name, "vlan"):
		return "vlan"
	case strings.HasPrefix(name, "eth"):
		return "ethernet"
	default:
		if linkType == "loopback" {
			return "loopback"
		}
		return "unknown"
	}
}

// --- Routes ------------------------------------------------------------

type ipRouteEntry struct {
	Dst     string `json:"dst"`
	Gateway string `json:"gateway"`
	Dev     string `json:"dev"`
	Metric  int    `json:"metric"`
}

func (b *Backend) Routes(ctx context.Context, table string) ([]backend.Route, error) {
	args := []string{"-j", "route"}
	if table != "" {
		args = append(args, "show", "table", table)
	}
	out, err := b.run(ctx, 5*time.Second, "ip", args...)
	if err != nil {
		return nil, fmt.Errorf("asuswrt: ip route: %w", err)
	}
	var entries []ipRouteEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, fmt.Errorf("asuswrt: parsing ip -j route output: %w", err)
	}

	tableName := table
	if tableName == "" {
		tableName = "main"
	}
	routes := make([]backend.Route, 0, len(entries))
	for _, e := range entries {
		dst := e.Dst
		if dst == "" {
			dst = "default"
		}
		routes = append(routes, backend.Route{
			Table:       tableName,
			Destination: dst,
			Gateway:     e.Gateway,
			Interface:   e.Dev,
			Metric:      e.Metric,
		})
	}
	return routes, nil
}

// --- Clients -------------------------------------------------------------

type ipNeighEntry struct {
	Dst    string   `json:"dst"`
	Lladdr string   `json:"lladdr"`
	Dev    string   `json:"dev"`
	State  []string `json:"state"`
}

func (b *Backend) Clients(ctx context.Context) ([]backend.Client, error) {
	neighOut, err := b.run(ctx, 5*time.Second, "ip", "-j", "neigh")
	if err != nil {
		return nil, fmt.Errorf("asuswrt: ip -j neigh: %w", err)
	}
	var neighbors []ipNeighEntry
	if err := json.Unmarshal([]byte(neighOut), &neighbors); err != nil {
		return nil, fmt.Errorf("asuswrt: parsing ip -j neigh output: %w", err)
	}

	// (verify) dnsmasq leases file path on this Merlin build — commonly
	// /var/lib/misc/dnsmasq.leases but Merlin sometimes uses
	// /tmp/var/lib/misc or a JFFS path; confirm on-device and adjust.
	hostnames := readDnsmasqLeases("/var/lib/misc/dnsmasq.leases")

	clients := make([]backend.Client, 0, len(neighbors))
	for _, n := range neighbors {
		if n.Lladdr == "" {
			continue
		}
		reachable := false
		for _, s := range n.State {
			if s == "REACHABLE" || s == "STALE" || s == "DELAY" || s == "PERMANENT" {
				reachable = true
			}
		}
		if !reachable {
			continue
		}
		c := backend.Client{
			MAC:       strings.ToUpper(n.Lladdr),
			IP:        n.Dst,
			Interface: n.Dev,
			Wireless:  strings.HasPrefix(n.Dev, "wl") || strings.HasPrefix(n.Dev, "ath"),
			LastSeen:  time.Now(),
		}
		if lease, ok := hostnames[strings.ToLower(n.Lladdr)]; ok {
			c.Hostname = lease.hostname
			if !lease.expires.IsZero() {
				c.LeaseExpires = lease.expires
			}
		}
		clients = append(clients, c)
	}
	return clients, nil
}

type leaseInfo struct {
	hostname string
	expires  time.Time
}

func readDnsmasqLeases(path string) map[string]leaseInfo {
	out := make(map[string]leaseInfo)
	f, err := os.Open(path)
	if err != nil {
		return out // best-effort: no leases file is not fatal
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// dnsmasq.leases format: <expiry-epoch> <mac> <ip> <hostname> <client-id>
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		mac := strings.ToLower(fields[1])
		hostname := fields[3]
		if hostname == "*" {
			hostname = ""
		}
		li := leaseInfo{hostname: hostname}
		if epoch, err := strconv.ParseInt(fields[0], 10, 64); err == nil && epoch > 0 {
			li.expires = time.Unix(epoch, 0)
		}
		out[mac] = li
	}
	return out
}

// --- WiFiStatus ------------------------------------------------------------

// (verify) this entire method against real `wl` output — the wl(8) tool's
// text output format is not stable across Broadcom SDK versions, and this
// is a best-effort parse based on commonly documented `wl status` /
// `wl assoclist` behavior. wifiInterfaces below must also be confirmed
// against docs/hardware.md's device-facts capture (radio -> wlN mapping).
var wifiInterfaces = []struct {
	Iface string
	Band  string
}{
	{"wl0", "2.4GHz"}, // (verify)
	{"wl1", "5GHz"},   // (verify)
}

func (b *Backend) WiFiStatus(ctx context.Context) ([]backend.WiFiRadio, error) {
	radios := make([]backend.WiFiRadio, 0, len(wifiInterfaces))
	for i, w := range wifiInterfaces {
		radio := backend.WiFiRadio{Interface: w.Iface, Band: w.Band}

		ssid, _ := b.nvramGet(ctx, fmt.Sprintf("wl%d_ssid", i))
		radio.SSID = ssid

		radioOn, _ := b.nvramGet(ctx, fmt.Sprintf("wl%d_radio", i))
		radio.Enabled = radioOn != "0"

		chanspec, _ := b.nvramGet(ctx, fmt.Sprintf("wl%d_chanspec", i))
		radio.Channel, radio.ChannelWidthMHz = parseChanspec(chanspec)

		assocOut, err := b.run(ctx, 5*time.Second, "wl", "-i", w.Iface, "assoclist")
		if err == nil {
			radio.ClientCount = strings.Count(assocOut, "assoclist")
			// wl assoclist typically prints one "assoclist <mac>" line per
			// client; count lines starting with that token instead of a
			// substring count once the real format is confirmed.
			radio.ClientCount = countAssocLines(assocOut)
		}

		radios = append(radios, radio)
	}
	return radios, nil
}

func countAssocLines(out string) int {
	n := 0
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if strings.HasPrefix(strings.TrimSpace(sc.Text()), "assoclist") {
			n++
		}
	}
	return n
}

// parseChanspec best-effort parses a Broadcom chanspec string like
// "36/80" (channel/width) or a bare channel number. (verify) against real
// nvram values — chanspec encoding varies and a bitfield-accurate parser may
// be needed instead of this string split.
func parseChanspec(spec string) (channel int, widthMHz int) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0
	}
	parts := strings.SplitN(spec, "/", 2)
	ch, _ := strconv.Atoi(strings.TrimRight(parts[0], "ul")) // strip u/l sideband suffixes if present
	channel = ch
	if len(parts) == 2 {
		width, _ := strconv.Atoi(parts[1])
		widthMHz = width
	} else {
		widthMHz = 20
	}
	return channel, widthMHz
}

// --- ShellExec ---------------------------------------------------------

func (b *Backend) ShellExec(ctx context.Context, command string, timeoutSeconds int) (backend.ShellResult, error) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	start := time.Now()
	stdout, stderr, code, timedOut, err := b.runner.run(ctx, timeout, "sh", "-c", command)
	res := backend.ShellResult{
		Command:  command,
		ExitCode: code,
		Stdout:   stdout,
		Stderr:   stderr,
		Duration: time.Since(start),
		TimedOut: timedOut,
	}
	if err != nil && !timedOut {
		return res, fmt.Errorf("asuswrt: shell_exec: %w", err)
	}
	return res, nil
}

// --- Backup / Restore ----------------------------------------------------
//
// (verify) This uses `nvram show` piped to a file as the settings snapshot,
// which captures the full nvram key/value set but is NOT the same format
// the ASUS web UI's "Save Settings" produces (that uses a proprietary
// .CFG encoding). For Milestone 2 this is sufficient for AXOS's own
// rollback engine (it restores via `nvram set`+`commit`, not via the web
// UI's restore path); it should NOT be assumed interchangeable with a
// web-UI-exported settings file without further verification.

func (b *Backend) Backup(ctx context.Context, reason string) (backend.BackupInfo, error) {
	if err := os.MkdirAll(b.BackupDir, 0750); err != nil {
		return backend.BackupInfo{}, fmt.Errorf("asuswrt: creating backup dir %s: %w", b.BackupDir, err)
	}

	out, err := b.run(ctx, 15*time.Second, "nvram", "show")
	if err != nil {
		return backend.BackupInfo{}, fmt.Errorf("asuswrt: nvram show: %w", err)
	}

	id := fmt.Sprintf("backup-%s", time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(b.BackupDir, id+".nvram.txt")
	if err := os.WriteFile(path, []byte(out), 0640); err != nil {
		return backend.BackupInfo{}, fmt.Errorf("asuswrt: writing backup %s: %w", path, err)
	}

	sum := sha256.Sum256([]byte(out))
	info := backend.BackupInfo{
		ID:        id,
		Path:      path,
		SizeBytes: int64(len(out)),
		SHA256:    hex.EncodeToString(sum[:]),
		CreatedAt: time.Now(),
		Reason:    reason,
	}

	metaPath := filepath.Join(b.BackupDir, id+".meta.json")
	metaBytes, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(metaPath, metaBytes, 0640); err != nil {
		return info, fmt.Errorf("asuswrt: writing backup metadata %s: %w", metaPath, err)
	}

	return info, nil
}

func (b *Backend) Restore(ctx context.Context, backupID string) error {
	metaPath := filepath.Join(b.BackupDir, backupID+".meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("asuswrt: unknown backup %q: %w", backupID, err)
	}
	var info backend.BackupInfo
	if err := json.Unmarshal(metaBytes, &info); err != nil {
		return fmt.Errorf("asuswrt: corrupt backup metadata %s: %w", metaPath, err)
	}

	data, err := os.ReadFile(info.Path)
	if err != nil {
		return fmt.Errorf("asuswrt: reading backup file %s: %w", info.Path, err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != info.SHA256 {
		return fmt.Errorf("asuswrt: backup %s failed checksum verification — refusing to restore a corrupt snapshot", backupID)
	}

	// nvram show output is "key=value" per line, with a trailing "size: N
	// bytes" summary line to skip. Restoring by replaying `nvram set` for
	// every key is deliberately conservative (no bulk-import shortcut) so a
	// malformed line fails loudly instead of corrupting nvram state.
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	applied := 0
	for sc.Scan() {
		line := sc.Text()
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(kv[0])
		if key == "" || strings.Contains(key, " ") {
			continue // skip summary/non-kv lines like "size: 12345 bytes (...)"
		}
		val := kv[1]
		if _, err := b.run(ctx, 5*time.Second, "nvram", "set", key+"="+val); err != nil {
			return fmt.Errorf("asuswrt: restoring nvram key %q: %w (applied %d keys before failure)", key, err, applied)
		}
		applied++
	}

	if _, err := b.run(ctx, 10*time.Second, "nvram", "commit"); err != nil {
		return fmt.Errorf("asuswrt: nvram commit after restore: %w", err)
	}

	// A full restore typically requires restarting affected services or
	// rebooting for nvram changes to take effect everywhere. Left to the
	// caller (the rollback engine documents this — see
	// docs/safety-rollback.md "Subsystem snapshots" for the lighter-weight
	// alternative that avoids needing a reboot for common cases).
	return nil
}

func (b *Backend) ListBackups(ctx context.Context) ([]backend.BackupInfo, error) {
	entries, err := os.ReadDir(b.BackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("asuswrt: listing backup dir %s: %w", b.BackupDir, err)
	}

	var out []backend.BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(b.BackupDir, e.Name()))
		if err != nil {
			continue
		}
		var info backend.BackupInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

var _ backend.RouterBackend = (*Backend)(nil)

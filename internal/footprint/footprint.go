// Package footprint measures and records AXOS's own RAM usage over time —
// distinct from backend.Resources, which reports the router's whole-system
// memory. The point is to answer "how much RAM is AXOS itself adding on top
// of stock firmware, and is that growing as features get added" on a device
// with only 1GB of RAM total, where every extra megabyte matters.
//
// The intended usage pattern: axosd records a "baseline" snapshot
// automatically the first time it ever runs (before any AXOS feature has had
// a chance to grow the process), then a snapshot on every subsequent
// startup, so a plain restart-after-deploy (the normal hot-deploy loop, see
// docs/development.md) is enough to build a growth timeline with no extra
// manual step. axosctl/the Core API can also force an on-demand snapshot at
// any time.
package footprint

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// Snapshot is one point-in-time measurement, appended to a Store's log.
type Snapshot struct {
	Timestamp time.Time `json:"ts"`
	// Label identifies why this snapshot was taken: "baseline" (the very
	// first snapshot ever recorded for this install), "startup" (axosd
	// process start), "manual" (an explicit axosctl/API request), or a
	// caller-supplied value (e.g. a release id).
	Label     string `json:"label"`
	ReleaseID string `json:"release_id,omitempty"`

	// ProcessRSSKB is the resident set size of the measuring process itself
	// (axosd, or whichever AXOS process called Measure), in kB. This is the
	// number that matters for "how much RAM does AXOS cost" — it's not
	// available on non-Linux platforms (see rss_other.go), in which case
	// it's left 0.
	ProcessRSSKB uint64 `json:"process_rss_kb"`
	// GoHeapAllocKB / GoSysKB / NumGoroutine are Go runtime internals,
	// useful for diagnosing *why* ProcessRSSKB moved (a leak vs. expected
	// growth) but not the headline number themselves.
	GoHeapAllocKB uint64 `json:"go_heap_alloc_kb"`
	GoSysKB       uint64 `json:"go_sys_kb"`
	NumGoroutine  int    `json:"num_goroutine"`

	// System*KB are the router's whole-system memory (from
	// backend.Resources), included for context — "AXOS is using X out of Y
	// free" — not filled in by Measure itself (it has no RouterBackend);
	// callers with one attach it before calling Store.Record.
	SystemTotalKB uint64 `json:"system_total_kb,omitempty"`
	SystemUsedKB  uint64 `json:"system_used_kb,omitempty"`
	SystemFreeKB  uint64 `json:"system_free_kb,omitempty"`
}

// Measure captures a Snapshot of the calling process right now. label
// should be "baseline", "startup", or "manual" for the conventions above,
// or any other short string the caller finds useful; releaseID is optional
// (empty is fine) and is meant for tagging a snapshot with the AXOS release
// that was running when it was taken (see internal/deploy).
//
// ProcessRSSKB is best-effort: on a platform where it can't be read (see
// rss_other.go), it's left 0 rather than failing the whole measurement —
// the Go runtime stats are still useful on their own.
func Measure(label, releaseID string) Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	rss, _ := readProcessRSSKB()

	return Snapshot{
		Timestamp:     time.Now(),
		Label:         label,
		ReleaseID:     releaseID,
		ProcessRSSKB:  rss,
		GoHeapAllocKB: ms.HeapAlloc / 1024,
		GoSysKB:       ms.Sys / 1024,
		NumGoroutine:  runtime.NumGoroutine(),
	}
}

// Store is an append-only JSONL log of Snapshots, mirroring
// internal/audit's Logger: cheap to append to, trivial to read back and
// replay/scan. Safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	path  string
	f     *os.File
	nowFn func() time.Time // overridable in tests
}

// Open opens (creating if needed) an append-only footprint log at path.
// Unlike audit.Open, this file is not restricted to owner-only permissions
// (0600) — a footprint snapshot is just timestamps, labels, and memory
// figures, nothing sensitive (see docs/security.md's "audit log
// confidentiality" for why the audit log itself is different).
func Open(path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("footprint: open %s: %w", path, err)
	}
	return &Store{path: path, f: f, nowFn: time.Now}, nil
}

// Close closes the underlying file.
func (s *Store) Close() error {
	if s.f != nil {
		return s.f.Close()
	}
	return nil
}

// IsEmpty reports whether this store has no snapshots recorded yet — the
// signal callers use to decide whether the next Measure should be labelled
// "baseline".
func (s *Store) IsEmpty() bool {
	info, err := os.Stat(s.path)
	return err != nil || info.Size() == 0
}

// Record appends snap to the log, filling in Timestamp if it's zero.
func (s *Store) Record(snap Snapshot) error {
	if snap.Timestamp.IsZero() {
		snap.Timestamp = s.nowFn()
	}
	line, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("footprint: marshal snapshot: %w", err)
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.f.Write(line); err != nil {
		return fmt.Errorf("footprint: write snapshot: %w", err)
	}
	return nil
}

// History reads every snapshot recorded so far, oldest first. Reading is a
// fresh, independent open of the file (not the append-mode handle Record
// writes through), so it's safe to call while this or another process keeps
// appending.
func (s *Store) History() ([]Snapshot, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("footprint: reading %s: %w", s.path, err)
	}
	defer f.Close()

	var out []Snapshot
	dec := json.NewDecoder(f)
	for dec.More() {
		var snap Snapshot
		if err := dec.Decode(&snap); err != nil {
			return out, fmt.Errorf("footprint: decoding %s: %w", s.path, err)
		}
		out = append(out, snap)
	}
	return out, nil
}

// Baseline returns the snapshot to compare growth against: the first entry
// labelled "baseline" if one exists, otherwise the very first entry ever
// recorded. ok is false only if the store has no entries at all.
func (s *Store) Baseline() (snap Snapshot, ok bool, err error) {
	hist, err := s.History()
	if err != nil {
		return Snapshot{}, false, err
	}
	for _, h := range hist {
		if h.Label == "baseline" {
			return h, true, nil
		}
	}
	if len(hist) > 0 {
		return hist[0], true, nil
	}
	return Snapshot{}, false, nil
}

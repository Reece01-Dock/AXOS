package footprint

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestMeasure_FillsGoRuntimeStats(t *testing.T) {
	snap := Measure("manual", "")
	if snap.Timestamp.IsZero() {
		t.Error("Timestamp not set")
	}
	if snap.NumGoroutine <= 0 {
		t.Error("NumGoroutine should be > 0 (this test itself is a goroutine)")
	}
	if snap.GoSysKB == 0 {
		t.Error("GoSysKB should be > 0 — the Go runtime has always reserved some memory by this point")
	}
	if runtime.GOOS == "linux" && snap.ProcessRSSKB == 0 {
		t.Error("ProcessRSSKB should be > 0 on linux")
	}
}

func TestStore_RecordAndHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "footprint.jsonl")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if !store.IsEmpty() {
		t.Fatal("a freshly opened store over a nonexistent file should be empty")
	}

	first := Measure("baseline", "")
	if err := store.Record(first); err != nil {
		t.Fatalf("Record baseline: %v", err)
	}
	if store.IsEmpty() {
		t.Fatal("store should no longer be empty after a Record")
	}

	second := Measure("startup", "000002")
	if err := store.Record(second); err != nil {
		t.Fatalf("Record second: %v", err)
	}

	hist, err := store.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("History returned %d entries, want 2", len(hist))
	}
	if hist[0].Label != "baseline" || hist[1].Label != "startup" {
		t.Errorf("History order/labels = %q, %q, want baseline, startup", hist[0].Label, hist[1].Label)
	}
	if hist[1].ReleaseID != "000002" {
		t.Errorf("second snapshot ReleaseID = %q, want 000002", hist[1].ReleaseID)
	}
}

func TestStore_Baseline_PrefersLabelledEntry(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "footprint.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	_ = store.Record(Measure("baseline", ""))
	_ = store.Record(Measure("startup", ""))
	_ = store.Record(Measure("startup", ""))

	base, ok, err := store.Baseline()
	if err != nil {
		t.Fatalf("Baseline: %v", err)
	}
	if !ok {
		t.Fatal("Baseline should be found")
	}
	if base.Label != "baseline" {
		t.Errorf("Baseline label = %q, want baseline", base.Label)
	}
}

func TestStore_Baseline_FallsBackToFirstEntry(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "footprint.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	// No "baseline"-labelled entry at all — e.g. an install that predates
	// this feature. Baseline should still fall back sensibly.
	_ = store.Record(Measure("startup", ""))
	_ = store.Record(Measure("startup", ""))

	base, ok, err := store.Baseline()
	if err != nil {
		t.Fatalf("Baseline: %v", err)
	}
	if !ok {
		t.Fatal("Baseline should be found")
	}
	if base.Timestamp.IsZero() {
		t.Error("fallback baseline should still be a real snapshot")
	}
}

func TestStore_Baseline_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "footprint.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	_, ok, err := store.Baseline()
	if err != nil {
		t.Fatalf("Baseline: %v", err)
	}
	if ok {
		t.Error("Baseline should report ok=false for an empty store")
	}
}

func TestStore_History_NonexistentFileIsEmptyNotError(t *testing.T) {
	dir := t.TempDir()
	// A Store that was opened (which creates the file) always has a file on
	// disk, so exercise History's not-exist branch directly against a path
	// nothing has touched.
	store := &Store{path: filepath.Join(dir, "never-created.jsonl")}
	hist, err := store.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if hist != nil {
		t.Errorf("History = %v, want nil", hist)
	}
}

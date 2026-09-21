package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeStagingBuild populates a Deployer's staging dir with a trivial fake
// build (one file) and returns a matching Manifest — the smallest possible
// stand-in for "a real cross-compiled axosd binary plus assets".
func writeStagingBuild(t *testing.T, d *Deployer, content string) *Manifest {
	t.Helper()
	if err := d.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	binPath := filepath.Join(d.StagingDir(), "bin", "axosd")
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(binPath, []byte(content), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := BuildManifest(d.StagingDir(), "abc123", []string{"axosd"})
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	return m
}

func TestDeploy_FirstDeployHasNoPrevious(t *testing.T) {
	d := New(t.TempDir())
	m := writeStagingBuild(t, d, "build 1")

	res, err := d.Deploy(context.Background(), m)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if res.ReleaseID != "000001" {
		t.Errorf("ReleaseID = %q, want 000001", res.ReleaseID)
	}
	if res.PreviousReleaseID != "" {
		t.Errorf("PreviousReleaseID = %q, want empty on first deploy", res.PreviousReleaseID)
	}

	cur, err := d.CurrentRelease()
	if err != nil || cur != "000001" {
		t.Fatalf("CurrentRelease() = %q, %v; want 000001", cur, err)
	}

	// staging should be empty again, ready for the next build.
	entries, _ := os.ReadDir(d.StagingDir())
	if len(entries) != 0 {
		t.Errorf("staging dir has %d entries after deploy, want 0", len(entries))
	}

	// The deployed file should actually be there under current.
	data, err := os.ReadFile(filepath.Join(d.Root, "releases", "000001", "bin", "axosd"))
	if err != nil || string(data) != "build 1" {
		t.Errorf("deployed file = %q, %v; want %q", data, err, "build 1")
	}
}

func TestDeploy_SecondDeploySetsPrevious(t *testing.T) {
	d := New(t.TempDir())

	m1 := writeStagingBuild(t, d, "build 1")
	if _, err := d.Deploy(context.Background(), m1); err != nil {
		t.Fatalf("first Deploy: %v", err)
	}

	m2 := writeStagingBuild(t, d, "build 2")
	res2, err := d.Deploy(context.Background(), m2)
	if err != nil {
		t.Fatalf("second Deploy: %v", err)
	}
	if res2.ReleaseID != "000002" {
		t.Errorf("second ReleaseID = %q, want 000002", res2.ReleaseID)
	}
	if res2.PreviousReleaseID != "000001" {
		t.Errorf("second PreviousReleaseID = %q, want 000001", res2.PreviousReleaseID)
	}

	prev, err := d.PreviousRelease()
	if err != nil || prev != "000001" {
		t.Fatalf("PreviousRelease() = %q, %v; want 000001", prev, err)
	}
	cur, err := d.CurrentRelease()
	if err != nil || cur != "000002" {
		t.Fatalf("CurrentRelease() = %q, %v; want 000002", cur, err)
	}
}

func TestDeploy_RefusesEmptyStaging(t *testing.T) {
	d := New(t.TempDir())
	if err := d.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	m := &Manifest{Files: map[string]string{}}
	if _, err := d.Deploy(context.Background(), m); err == nil {
		t.Fatal("Deploy with empty staging should fail")
	}
}

func TestDeploy_RefusesChecksumMismatch(t *testing.T) {
	d := New(t.TempDir())
	m := writeStagingBuild(t, d, "build 1")

	// Tamper with the staged file after the manifest was computed.
	binPath := filepath.Join(d.StagingDir(), "bin", "axosd")
	if err := os.WriteFile(binPath, []byte("tampered content"), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := d.Deploy(context.Background(), m); err == nil {
		t.Fatal("Deploy with a checksum mismatch should fail")
	}

	// And it must not have promoted anything.
	if cur, _ := d.CurrentRelease(); cur != "" {
		t.Errorf("CurrentRelease() = %q after a refused deploy, want empty", cur)
	}
}

func TestDeploy_RefusesMissingManifestFile(t *testing.T) {
	d := New(t.TempDir())
	writeStagingBuild(t, d, "build 1")

	// A manifest that references a file that doesn't exist in staging.
	badManifest := &Manifest{Files: map[string]string{"bin/does-not-exist": "deadbeef"}}
	if _, err := d.Deploy(context.Background(), badManifest); err == nil {
		t.Fatal("Deploy with a manifest referencing a missing file should fail")
	}
}

func TestRollback_ToPrevious(t *testing.T) {
	d := New(t.TempDir())
	m1 := writeStagingBuild(t, d, "build 1")
	d.Deploy(context.Background(), m1)
	m2 := writeStagingBuild(t, d, "build 2")
	d.Deploy(context.Background(), m2)

	res, err := d.Rollback(context.Background(), "")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if res.ReleaseID != "000001" {
		t.Errorf("Rollback ReleaseID = %q, want 000001", res.ReleaseID)
	}

	cur, _ := d.CurrentRelease()
	if cur != "000001" {
		t.Errorf("CurrentRelease() after rollback = %q, want 000001", cur)
	}
	prev, _ := d.PreviousRelease()
	if prev != "000002" {
		t.Errorf("PreviousRelease() after rollback = %q, want 000002 (rollback should be itself reversible)", prev)
	}

	data, _ := os.ReadFile(filepath.Join(d.Root, "current", "bin", "axosd"))
	if string(data) != "build 1" {
		t.Errorf("current/bin/axosd after rollback = %q, want %q", data, "build 1")
	}
}

func TestRollback_ToExplicitVersion(t *testing.T) {
	d := New(t.TempDir())
	for i := 0; i < 3; i++ {
		m := writeStagingBuild(t, d, "build")
		if _, err := d.Deploy(context.Background(), m); err != nil {
			t.Fatalf("Deploy %d: %v", i, err)
		}
	}
	// current=000003, previous=000002

	res, err := d.Rollback(context.Background(), "000001")
	if err != nil {
		t.Fatalf("Rollback to 000001: %v", err)
	}
	if res.ReleaseID != "000001" {
		t.Errorf("ReleaseID = %q, want 000001", res.ReleaseID)
	}
	cur, _ := d.CurrentRelease()
	if cur != "000001" {
		t.Errorf("CurrentRelease() = %q, want 000001", cur)
	}
}

func TestRollback_UnknownVersion(t *testing.T) {
	d := New(t.TempDir())
	m := writeStagingBuild(t, d, "build 1")
	if _, err := d.Deploy(context.Background(), m); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := d.Rollback(context.Background(), "999999"); err == nil {
		t.Fatal("Rollback to a nonexistent version should fail")
	}
}

func TestRollback_NoPreviousAvailable(t *testing.T) {
	d := New(t.TempDir())
	m := writeStagingBuild(t, d, "build 1")
	if _, err := d.Deploy(context.Background(), m); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := d.Rollback(context.Background(), ""); err == nil {
		t.Fatal("Rollback with no previous release and no explicit version should fail")
	}
}

func TestHistory_ReflectsCurrentAndPrevious(t *testing.T) {
	d := New(t.TempDir())
	m1 := writeStagingBuild(t, d, "build 1")
	d.Deploy(context.Background(), m1)
	m2 := writeStagingBuild(t, d, "build 2")
	d.Deploy(context.Background(), m2)

	hist, err := d.History(context.Background())
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("History() = %d entries, want 2", len(hist))
	}
	// newest first
	if hist[0].ID != "000002" || !hist[0].IsCurrent || hist[0].IsPrevious {
		t.Errorf("hist[0] = %+v, want 000002 current=true previous=false", hist[0])
	}
	if hist[1].ID != "000001" || hist[1].IsCurrent || !hist[1].IsPrevious {
		t.Errorf("hist[1] = %+v, want 000001 current=false previous=true", hist[1])
	}
	if hist[0].Manifest == nil || hist[0].Manifest.Commit != "abc123" {
		t.Errorf("hist[0].Manifest = %+v, want commit abc123", hist[0].Manifest)
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("A"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("B"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	a, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil || string(a) != "A" {
		t.Errorf("dst/a.txt = %q, %v", a, err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil || string(b) != "B" {
		t.Errorf("dst/sub/b.txt = %q, %v", b, err)
	}
}

func TestManifest_VerifyDetectsMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := BuildManifest(dir, "c1", nil)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if err := m.Verify(dir); err != nil {
		t.Fatalf("Verify on unmodified dir should pass, got: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.Verify(dir); err == nil {
		t.Fatal("Verify after modifying a file should fail")
	}
}

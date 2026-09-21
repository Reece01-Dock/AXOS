// Package deploy implements AXOS's atomic release layout and hot-deploy
// mechanics: staging -> releases/NNNNNN -> current/previous symlinks, with
// checksum-verified promotion and instant rollback, all via atomic
// filesystem renames — no router reboot, no partially-applied release ever
// becomes "current".
//
// This package only manipulates the filesystem layout; it doesn't know
// about services or processes (see internal/supervisor for that) or how
// bytes get into staging/ in the first place (see cmd/axosctl's "deploy"
// command, which populates staging via local copy or SSH/rsync to a remote
// Root before calling Deploy).
package deploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Deployer manages the release layout rooted at Root (e.g. /opt/axos, or a
// temp dir in tests):
//
//	Root/
//	├── releases/000001/ 000002/ ...
//	├── current  -> releases/000004  (symlink)
//	├── previous -> releases/000003  (symlink)
//	└── staging/                      (populated by the caller, consumed by Deploy)
type Deployer struct {
	Root string
}

// New returns a Deployer rooted at root. It does not create any
// directories — call EnsureLayout for that (or just call Deploy, which
// creates what it needs).
func New(root string) *Deployer {
	return &Deployer{Root: root}
}

func (d *Deployer) releasesDir() string  { return filepath.Join(d.Root, "releases") }
func (d *Deployer) currentLink() string  { return filepath.Join(d.Root, "current") }
func (d *Deployer) previousLink() string { return filepath.Join(d.Root, "previous") }

// StagingDir is where the caller should place (copy, rsync, extract...) the
// build about to be deployed, before calling Deploy.
func (d *Deployer) StagingDir() string { return filepath.Join(d.Root, "staging") }

// EnsureLayout creates releases/ and staging/ if they don't already exist.
// Safe to call repeatedly.
func (d *Deployer) EnsureLayout() error {
	for _, dir := range []string{d.releasesDir(), d.StagingDir()} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("deploy: creating %s: %w", dir, err)
		}
	}
	return nil
}

// ReleaseInfo describes one entry under releases/.
type ReleaseInfo struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	CreatedAt  time.Time `json:"created_at"`
	IsCurrent  bool      `json:"is_current"`
	IsPrevious bool      `json:"is_previous"`
	Manifest   *Manifest `json:"manifest,omitempty"`
}

// Result is returned by a successful Deploy.
type Result struct {
	ReleaseID         string `json:"release_id"`
	Path              string `json:"path"`
	PreviousReleaseID string `json:"previous_release_id,omitempty"`
}

// Deploy promotes the contents of StagingDir() to a new release and
// atomically switches current to point at it:
//
//  1. verify staging is non-empty and matches manifest (checksums)
//  2. allocate the next release id
//  3. move staging -> releases/<id>  (a same-filesystem rename: atomic,
//     and it empties staging/ for next time as a side effect)
//  4. write the manifest into the release dir
//  5. atomically swap current -> releases/<id> (rename-over-symlink)
//  6. atomically swap previous -> whatever current pointed to before this
//
// Nothing here restarts any service — that's the caller's job (typically:
// call Deploy, then internal/supervisor.Restart for the affected services,
// then health-check, then Confirm-equivalent or Rollback). Deploy itself
// has no "undo": if you need to undo a Deploy, call Rollback.
func (d *Deployer) Deploy(ctx context.Context, manifest *Manifest) (Result, error) {
	if err := d.EnsureLayout(); err != nil {
		return Result{}, err
	}

	staging := d.StagingDir()
	entries, err := os.ReadDir(staging)
	if err != nil {
		return Result{}, fmt.Errorf("deploy: reading staging dir %s: %w", staging, err)
	}
	if len(entries) == 0 {
		return Result{}, fmt.Errorf("deploy: staging dir %s is empty — nothing to deploy", staging)
	}

	if manifest == nil {
		return Result{}, fmt.Errorf("deploy: manifest is required")
	}
	if err := manifest.Verify(staging); err != nil {
		return Result{}, err
	}

	id, err := d.nextReleaseID()
	if err != nil {
		return Result{}, err
	}
	target := filepath.Join(d.releasesDir(), id)

	if err := writeManifest(staging, manifest); err != nil {
		return Result{}, fmt.Errorf("deploy: writing manifest into staging: %w", err)
	}

	if err := os.Rename(staging, target); err != nil {
		return Result{}, fmt.Errorf("deploy: promoting staging to release %s: %w", id, err)
	}
	// staging/ is gone now (renamed away) — recreate an empty one so the
	// next Deploy's caller has somewhere to write to without extra steps.
	if err := os.MkdirAll(staging, 0755); err != nil {
		return Result{}, fmt.Errorf("deploy: recreating empty staging dir after promotion: %w", err)
	}

	prevID, _ := d.readLinkTargetID(d.currentLink())

	if err := d.atomicSymlink(d.currentLink(), target); err != nil {
		return Result{}, fmt.Errorf("deploy: switching current to release %s: %w", id, err)
	}
	if prevID != "" {
		if err := d.atomicSymlink(d.previousLink(), filepath.Join(d.releasesDir(), prevID)); err != nil {
			return Result{}, fmt.Errorf("deploy: updating previous pointer: %w", err)
		}
	}

	return Result{ReleaseID: id, Path: target, PreviousReleaseID: prevID}, nil
}

// Rollback atomically switches current back to a prior release: toVersion,
// if given, or whatever previous currently points at otherwise. This is
// instant (one symlink swap) — no re-copy, no rebuild.
func (d *Deployer) Rollback(ctx context.Context, toVersion string) (Result, error) {
	var targetID string
	if toVersion != "" {
		targetID = toVersion
	} else {
		id, err := d.readLinkTargetID(d.previousLink())
		if err != nil {
			return Result{}, fmt.Errorf("deploy: no previous release to roll back to (and no --version given): %w", err)
		}
		targetID = id
	}

	target := filepath.Join(d.releasesDir(), targetID)
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("deploy: release %s does not exist under %s", targetID, d.releasesDir())
	}

	curID, _ := d.readLinkTargetID(d.currentLink())

	if err := d.atomicSymlink(d.currentLink(), target); err != nil {
		return Result{}, fmt.Errorf("deploy: rolling back current to release %s: %w", targetID, err)
	}
	if curID != "" {
		if err := d.atomicSymlink(d.previousLink(), filepath.Join(d.releasesDir(), curID)); err != nil {
			return Result{}, fmt.Errorf("deploy: updating previous pointer after rollback: %w", err)
		}
	}

	return Result{ReleaseID: targetID, Path: target, PreviousReleaseID: curID}, nil
}

// History lists every release under releases/, newest first, annotated
// with whether it's the current and/or previous release.
func (d *Deployer) History(ctx context.Context) ([]ReleaseInfo, error) {
	entries, err := os.ReadDir(d.releasesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("deploy: reading %s: %w", d.releasesDir(), err)
	}

	curID, _ := d.readLinkTargetID(d.currentLink())
	prevID, _ := d.readLinkTargetID(d.previousLink())

	var out []ReleaseInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		ri := ReleaseInfo{
			ID:         e.Name(),
			Path:       filepath.Join(d.releasesDir(), e.Name()),
			CreatedAt:  info.ModTime(),
			IsCurrent:  e.Name() == curID,
			IsPrevious: e.Name() == prevID,
		}
		if m, err := readManifest(ri.Path); err == nil {
			ri.Manifest = m
		}
		out = append(out, ri)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// CurrentRelease returns the release id current points at, or "" if unset.
func (d *Deployer) CurrentRelease() (string, error) {
	return d.readLinkTargetID(d.currentLink())
}

// PreviousRelease returns the release id previous points at, or "" if unset.
func (d *Deployer) PreviousRelease() (string, error) {
	return d.readLinkTargetID(d.previousLink())
}

func (d *Deployer) nextReleaseID() (string, error) {
	entries, err := os.ReadDir(d.releasesDir())
	if err != nil {
		return "", fmt.Errorf("deploy: reading %s: %w", d.releasesDir(), err)
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if n, err := strconv.Atoi(e.Name()); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%06d", max+1), nil
}

// readLinkTargetID resolves a symlink (current/previous) and returns just
// the release id (the final path component), not the full path. Returns
// ("", err) if the link doesn't exist — not treated as fatal by callers,
// since "no previous release yet" is normal for a first deploy.
func (d *Deployer) readLinkTargetID(link string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	return filepath.Base(target), nil
}

// atomicSymlink points link at target without ever leaving link missing or
// pointing at a half-written path: create a new symlink at a temp name
// next to link, then rename it over link. os.Rename is atomic on POSIX
// even when the destination already exists (and even when it's a symlink),
// which is the entire property this function exists to provide.
func (d *Deployer) atomicSymlink(link, target string) error {
	tmp := link + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// CopyDir recursively copies src into dst (creating dst if needed) —
// a convenience for populating StagingDir() from a local build output
// directory. Symlinks in src are followed (their target's content is
// copied), not preserved as symlinks, since a build output tree shouldn't
// contain meaningful symlinks of its own.
func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

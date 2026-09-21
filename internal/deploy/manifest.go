package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Manifest describes one build's contents: what it is, when it was built,
// and a checksum for every file it contains. Deploy refuses to promote a
// staged release whose files don't match its manifest — this is what "no
// partial/corrupt release ever becomes current" actually means in code,
// not just in the design doc.
type Manifest struct {
	Commit     string            `json:"commit"`
	BuiltAt    time.Time         `json:"built_at"`
	Components []string          `json:"components"` // e.g. ["axosd", "axos-mcp"] — informational, what changed
	Files      map[string]string `json:"files"`      // relative path -> sha256 hex
}

// BuildManifest walks dir and computes a Manifest for its current contents.
// Typically called by a build script right after producing the staged
// output, with commit set to the VCS revision being built (pass "unknown"
// if unavailable — never fabricate one).
func BuildManifest(dir, commit string, components []string) (*Manifest, error) {
	m := &Manifest{Commit: commit, BuiltAt: time.Now(), Components: components, Files: make(map[string]string)}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == manifestFileName {
			return nil // never include the manifest file in its own checksum set
		}
		sum, err := sha256File(path)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", rel, err)
		}
		m.Files[rel] = sum
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("deploy: building manifest for %s: %w", dir, err)
	}
	return m, nil
}

// Verify re-hashes every file dir contains that the manifest lists, and
// fails on the first mismatch or missing file. It does NOT fail on extra
// files present in dir but absent from the manifest (a build tool adding
// an incidental file, e.g. a .DS_Store, shouldn't block a deploy) — only
// missing or altered files are load-bearing here.
func (m *Manifest) Verify(dir string) error {
	names := make([]string, 0, len(m.Files))
	for rel := range m.Files {
		names = append(names, rel)
	}
	sort.Strings(names) // deterministic error ordering

	for _, rel := range names {
		wantSum := m.Files[rel]
		path := filepath.Join(dir, rel)
		gotSum, err := sha256File(path)
		if err != nil {
			return fmt.Errorf("deploy: manifest verification failed: %s: %w", rel, err)
		}
		if gotSum != wantSum {
			return fmt.Errorf("deploy: manifest verification failed: %s: checksum mismatch (want %s, got %s)", rel, wantSum, gotSum)
		}
	}
	return nil
}

const manifestFileName = "MANIFEST.json"

func writeManifest(dir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, manifestFileName), data, 0644)
}

func readManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFileName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

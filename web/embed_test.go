package web

import (
	"io/fs"
	"testing"
)

func TestEmbeddedAssetsPresent(t *testing.T) {
	for _, name := range []string{"index.html", "styles.css", "app.js"} {
		f, err := FS.Open(name)
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		_ = f.Close()
	}
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected >=3 embedded files, got %d", len(entries))
	}
}

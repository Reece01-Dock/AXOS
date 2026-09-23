package api

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// SetUI configures the Phase 7 static asset filesystem served at GET / and
// GET /ui/*. Call before the HTTP server starts accepting requests.
// cmd/axosd passes os.DirFS(-ui-dir) when that directory exists, otherwise
// the embedded web.FS. When unset, UI routes respond 404.
func (s *Server) SetUI(fsys fs.FS) {
	s.uiFS = fsys
}

func (s *Server) registerUIRoutes() {
	s.mux.HandleFunc("GET /{$}", s.handleUIIndex)
	s.mux.HandleFunc("GET /ui/{path...}", s.handleUIStatic)
}

func (s *Server) handleUIIndex(w http.ResponseWriter, r *http.Request) {
	if s.uiFS == nil {
		http.NotFound(w, r)
		return
	}
	serveUIFile(w, r, s.uiFS, "index.html")
}

func (s *Server) handleUIStatic(w http.ResponseWriter, r *http.Request) {
	if s.uiFS == nil {
		http.NotFound(w, r)
		return
	}
	path := r.PathValue("path")
	if path == "" || strings.Contains(path, "..") {
		http.NotFound(w, r)
		return
	}
	serveUIFile(w, r, s.uiFS, path)
}

func serveUIFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	f, err := fsys.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		http.NotFound(w, r)
		return
	}

	// http.ServeContent needs an io.ReadSeeker; embed.FS and os.DirFS both
	// open as such for regular files.
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "ui file not seekable", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, stat.ModTime(), rs)
}

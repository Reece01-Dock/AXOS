// Package web ships the AXOS web UI static assets (Phase 7).
// Hot-deploy replaces these on the router under <axos-root>/www/; the
// embedded copy is the fallback when -ui-dir is empty.
package web

import "embed"

//go:embed index.html styles.css axos-ui.js
var FS embed.FS

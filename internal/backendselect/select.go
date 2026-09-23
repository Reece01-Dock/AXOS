// Package backendselect is the one place that turns "--backend X" style
// flags into a concrete backend.RouterBackend, shared by cmd/axosd and
// cmd/axosctl so the mock/replay/asuswrt selection logic (and its flags'
// meaning) can't drift between the two binaries.
package backendselect

import (
	"fmt"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/backend/asuswrt"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/backend/replay"
)

// Options carries every flag any backend might need. Irrelevant fields for
// a given Name are ignored (e.g. Fixture is ignored unless Name == "replay").
type Options struct {
	Name      string // "mock" | "replay" | "asuswrt"
	Fixture   string // replay: path to a fixture directory (see internal/capture)
	Host      string // asuswrt: if set, run over SSH via this host instead of locally
	SSHArgs   []string
	BackupDir string // asuswrt: override config.backup storage directory
}

// New builds the backend Options describes. Returns an error naming the
// valid choices if Name isn't one of them, so a typo in a flag fails
// immediately and clearly rather than as a confusing nil-backend panic
// later.
func New(o Options) (backend.RouterBackend, error) {
	switch o.Name {
	case "mock", "":
		return mock.New(), nil
	case "replay":
		if o.Fixture == "" {
			return nil, fmt.Errorf(`--backend replay requires --fixture <dir> (a directory written by "axosctl capture")`)
		}
		return replay.New(o.Fixture)
	case "asuswrt":
		var opts []asuswrt.Option
		if o.Host != "" {
			opts = append(opts, asuswrt.WithHost(o.Host, o.SSHArgs...))
		}
		if o.BackupDir != "" {
			opts = append(opts, asuswrt.WithBackupDir(o.BackupDir))
		}
		return asuswrt.New(opts...)
	default:
		return nil, fmt.Errorf(`unknown backend %q (want "mock", "replay", or "asuswrt")`, o.Name)
	}
}

//go:build !unix

package supervisor

import (
	"os"
	"os/exec"
)

// Non-Unix fallback (AXOS itself only ever runs on Linux — the router and
// any Linux dev machine — but this keeps `go build ./...` working on a
// contributor's Windows machine too). No process-group support here:
// signalProcessGroup falls back to signaling just the direct child.
func setProcessGroup(cmd *exec.Cmd) {}

func signalProcessGroup(pid int, kill bool) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if kill {
		_ = proc.Kill()
		return
	}
	_ = proc.Signal(os.Interrupt)
}

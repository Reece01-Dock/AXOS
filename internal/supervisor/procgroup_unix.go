//go:build unix

package supervisor

import (
	"os/exec"
	"syscall"
)

// setProcessGroup makes cmd the leader of its own process group (Setpgid),
// so signalProcessGroup can later signal it and everything it forked as a
// unit, rather than just the single direct child.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalProcessGroup sends SIGTERM (or SIGKILL if kill is true) to the
// process group led by pid — a negative pid in syscall.Kill targets the
// whole group rather than a single process. Errors are deliberately
// swallowed: this is a best-effort stop signal, and the caller (Stop) has
// its own poll-then-force-kill fallback regardless.
func signalProcessGroup(pid int, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}

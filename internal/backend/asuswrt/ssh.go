package asuswrt

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// sshRunner is a commandRunner that executes every command on a remote host
// via the local `ssh` binary, rather than locally via os/exec — this is
// what "Live Development Mode" (axosd --backend asuswrt --host <router>)
// means: same AsuswrtBackend, same nvram/ip/wl commands, just run over SSH
// instead of assuming the process is already running on the router.
//
// Shelling out to `ssh` rather than using an SSH client library is
// deliberate: it needs zero new dependencies, and it means whatever SSH
// config/known_hosts/agent setup already works for a human on this machine
// (ssh_config, ProxyJump, agent-forwarded keys, ...) works for axosd too,
// with no separate credential story to build or explain.
type sshRunner struct {
	sshBinary string   // usually "ssh"
	host      string   // "user@host" or "host" (rely on ~/.ssh/config for user)
	extraArgs []string // passed to ssh before the host, e.g. -F/-i/-p/-o overrides
}

// defaultSSHArgs enforces key-based, non-interactive auth: BatchMode=yes
// makes ssh fail immediately instead of prompting for a password if key
// auth doesn't work, rather than hanging or (worse) inviting a
// typed-password habit. This is the same posture docs/security.md's "MCP
// transport & access control" calls for — enforced here in code, not just
// documented.
var defaultSSHArgs = []string{
	"-o", "BatchMode=yes",
	"-o", "ConnectTimeout=8",
	"-o", "ServerAliveInterval=15",
}

// newSSHRunner builds a runner targeting host, using ssh's own config
// (~/.ssh/config, agent, known_hosts) for everything not explicitly
// overridden by extraArgs.
func newSSHRunner(host string, extraArgs ...string) *sshRunner {
	args := append(append([]string{}, defaultSSHArgs...), extraArgs...)
	return &sshRunner{sshBinary: "ssh", host: host, extraArgs: args}
}

func (r *sshRunner) run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, string, int, bool, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	argv := buildSSHArgv(r.sshBinary, r.extraArgs, r.host, name, args)
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	timedOut := cctx.Err() == context.DeadlineExceeded

	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			// Note: ssh itself exits 255 for connection-level failures
			// (auth, DNS, refused, ...) as distinct from the remote
			// command's own exit code — both surface here identically as a
			// non-zero exitCode, which b.run() turns into an error either
			// way. Distinguishing "couldn't reach the router" from "reached
			// it, command failed" is left to the caller reading stderr if
			// it matters, rather than special-cased here.
			exitCode = ee.ExitCode()
		} else if !timedOut {
			return stdout.String(), stderr.String(), -1, timedOut, err
		}
	}
	return stdout.String(), stderr.String(), exitCode, timedOut, nil
}

// buildSSHArgv constructs the full argv for running `name args...` on host
// over ssh: [sshBinary, extraArgs..., host, quotedRemoteCommand]. Split out
// as a pure function (no exec) so command construction/quoting is unit
// testable without a real SSH connection.
func buildSSHArgv(sshBinary string, extraArgs []string, host string, name string, args []string) []string {
	argv := make([]string, 0, len(extraArgs)+3)
	argv = append(argv, sshBinary)
	argv = append(argv, extraArgs...)
	argv = append(argv, host, shellJoin(name, args))
	return argv
}

// shellJoin quotes name and each arg for a POSIX remote shell and joins
// them with spaces — this is the single string ssh hands to the remote
// shell (`ssh host "the string"`), so an argument containing spaces or
// shell metacharacters must be quoted or it gets re-split/misinterpreted
// remotely.
func shellJoin(name string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, shellQuote(name))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps s in single quotes, escaping any embedded single quote
// as close-quote, backslash-escaped literal quote, reopen-quote — the
// standard POSIX-safe quoting technique that works for any input, including
// already-quoted shell fragments a caller might pass to system.shell_exec.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Package supervisor manages AXOS's hot-deployable services (axos-core,
// axos-api, axos-mcp, axos-monitor, ...) as OS processes: start, stop,
// restart, health check, and crash-restart with backoff. This is what
// "restart only the changed AXOS service" (docs/development.md) means in
// practice — internal/deploy switches which release's files are current;
// Supervisor is what actually stops the old process and starts the new one.
//
// Known limitation, stated plainly rather than glossed over: process state
// lives in the Supervisor's own memory. If the Supervisor process itself
// restarts, it loses track of any children it was watching (they keep
// running, orphaned, until something else notices). A production
// deployment needs a persistent supervisor (survives its own restart) —
// out of scope for this pass; axosd itself is the one process this
// project currently needs kept alive across restarts, and systemd/Merlin's
// own init handles that layer today. Revisit if/when axos-core grows
// enough hot-swappable sibling processes that this starts to matter.
package supervisor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ServiceSpec describes one supervised service.
type ServiceSpec struct {
	Name    string
	Command string // path to the binary to run
	Args    []string
	Env     []string // additional env vars (appended to the current process's env)

	// HealthURL, if set, is polled with GET for a health check (2xx =
	// healthy). If empty, health checking falls back to "is the process
	// still alive".
	HealthURL     string
	HealthTimeout time.Duration // default 5s if zero

	// RestartBackoffBase is the initial delay before restarting a crashed
	// process; it doubles on each consecutive crash, capped at
	// RestartBackoffMax. Defaults: 1s base, 30s max.
	RestartBackoffBase time.Duration
	RestartBackoffMax  time.Duration
	// MaxConsecutiveRestarts stops auto-restarting after this many crashes
	// in a row (reset to 0 once the process has run for
	// MinUptimeToResetRestartCount without crashing). Default 5, 0 = no
	// auto-restart at all (a genuinely one-shot service, rare).
	MaxConsecutiveRestarts       int
	MinUptimeToResetRestartCount time.Duration // default 60s
}

func (s ServiceSpec) withDefaults() ServiceSpec {
	if s.HealthTimeout == 0 {
		s.HealthTimeout = 5 * time.Second
	}
	if s.RestartBackoffBase == 0 {
		s.RestartBackoffBase = 1 * time.Second
	}
	if s.RestartBackoffMax == 0 {
		s.RestartBackoffMax = 30 * time.Second
	}
	if s.MaxConsecutiveRestarts == 0 {
		s.MaxConsecutiveRestarts = 5
	}
	if s.MinUptimeToResetRestartCount == 0 {
		s.MinUptimeToResetRestartCount = 60 * time.Second
	}
	return s
}

// State is a point-in-time snapshot of one service's status.
type State struct {
	Name                string    `json:"name"`
	Running             bool      `json:"running"`
	PID                 int       `json:"pid,omitempty"`
	StartedAt           time.Time `json:"started_at,omitempty"`
	Restarts            int       `json:"restarts"`
	LastExitCode        int       `json:"last_exit_code,omitempty"`
	LastExitAt          time.Time `json:"last_exit_at,omitempty"`
	GaveUpAfterRestarts bool      `json:"gave_up_after_restarts,omitempty"`
}

// Health is the result of a health check.
type Health struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail,omitempty"`
}

type managedProcess struct {
	mu   sync.Mutex
	spec ServiceSpec
	cmd  *exec.Cmd

	startedAt     time.Time
	restarts      int
	lastExitCode  int
	lastExitAt    time.Time
	stopRequested bool
	gaveUp        bool
	running       bool
}

// Supervisor manages a set of named services. Safe for concurrent use.
type Supervisor struct {
	mu    sync.Mutex
	procs map[string]*managedProcess
	// LogDir, if set, receives one <name>.log file per service (stdout+stderr,
	// appended). If empty, service output goes to Supervisor's own stdout/stderr.
	LogDir string
}

// New returns a ready-to-use Supervisor.
func New(logDir string) *Supervisor {
	return &Supervisor{procs: make(map[string]*managedProcess), LogDir: logDir}
}

// Register adds or replaces a service's spec. Does not start it — call
// Start explicitly (or use RegisterAndStart).
func (s *Supervisor) Register(spec ServiceSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec = spec.withDefaults()
	if p, exists := s.procs[spec.Name]; exists {
		p.mu.Lock()
		p.spec = spec
		p.mu.Unlock()
		return
	}
	s.procs[spec.Name] = &managedProcess{spec: spec}
}

func (s *Supervisor) get(name string) (*managedProcess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.procs[name]
	if !ok {
		return nil, fmt.Errorf("supervisor: unknown service %q", name)
	}
	return p, nil
}

// Start launches a registered service, if it isn't already running, and
// begins watching it for crashes (see the package doc's restart-with-backoff
// behavior).
func (s *Supervisor) Start(ctx context.Context, name string) error {
	p, err := s.get(name)
	if err != nil {
		return err
	}
	return s.startProcess(p)
}

func (s *Supervisor) startProcess(p *managedProcess) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("supervisor: %s is already running", p.spec.Name)
	}
	p.stopRequested = false
	p.gaveUp = false

	cmd := exec.Command(p.spec.Command, p.spec.Args...)
	cmd.Env = append(os.Environ(), p.spec.Env...)
	if out, err := s.serviceLogWriter(p.spec.Name); err == nil {
		cmd.Stdout, cmd.Stderr = out, out
	}
	// Run in its own process group so Stop can signal the whole group, not
	// just cmd.Process — a service specified as e.g. a shell script that
	// forks its own children would otherwise leave those children running
	// after "Stop" sends SIGTERM to only the shell.
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		p.mu.Unlock()
		return fmt.Errorf("supervisor: starting %s: %w", p.spec.Name, err)
	}
	p.cmd = cmd
	p.running = true
	p.startedAt = time.Now()
	p.mu.Unlock()

	go s.watch(p, cmd)
	return nil
}

// watch waits for the process to exit and, unless a Stop was requested or
// the service has exceeded MaxConsecutiveRestarts, restarts it after a
// backoff delay.
func (s *Supervisor) watch(p *managedProcess, cmd *exec.Cmd) {
	err := cmd.Wait()

	p.mu.Lock()
	p.running = false
	p.lastExitAt = time.Now()
	if ee, ok := err.(*exec.ExitError); ok {
		p.lastExitCode = ee.ExitCode()
	} else if err == nil {
		p.lastExitCode = 0
	}
	uptime := p.lastExitAt.Sub(p.startedAt)
	stopRequested := p.stopRequested
	p.mu.Unlock()

	if stopRequested {
		return
	}

	p.mu.Lock()
	if uptime >= p.spec.MinUptimeToResetRestartCount {
		p.restarts = 0
	}
	p.restarts++
	restarts := p.restarts
	backoff := p.spec.RestartBackoffBase << (restarts - 1)
	if backoff > p.spec.RestartBackoffMax || backoff <= 0 {
		backoff = p.spec.RestartBackoffMax
	}
	giveUp := restarts > p.spec.MaxConsecutiveRestarts
	if giveUp {
		p.gaveUp = true
	}
	p.mu.Unlock()

	if giveUp {
		return
	}

	time.Sleep(backoff)

	p.mu.Lock()
	stillWantsRestart := !p.stopRequested
	p.mu.Unlock()
	if stillWantsRestart {
		_ = s.startProcess(p)
	}
}

// Stop signals a service to exit (SIGTERM, then SIGKILL after timeout) and
// marks it as deliberately stopped, so watch() won't auto-restart it.
func (s *Supervisor) Stop(ctx context.Context, name string) error {
	p, err := s.get(name)
	if err != nil {
		return err
	}

	p.mu.Lock()
	if !p.running || p.cmd == nil || p.cmd.Process == nil {
		p.stopRequested = true
		p.mu.Unlock()
		return nil
	}
	pid := p.cmd.Process.Pid
	p.stopRequested = true
	p.mu.Unlock()

	// Signal the whole process group (see setProcessGroup in startProcess),
	// not just the direct child — that's what makes this reliably stop a
	// service even when Command is itself a wrapper (a shell script, a
	// launcher) that forked its own children.
	signalProcessGroup(pid, false)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		running := p.running
		p.mu.Unlock()
		if !running {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	signalProcessGroup(pid, true)
	return nil
}

// Restart stops (if running) and starts a service.
func (s *Supervisor) Restart(ctx context.Context, name string) error {
	if err := s.Stop(ctx, name); err != nil {
		return err
	}
	// Stop() waits for the process to actually exit (or force-kills it),
	// so watch()'s exit handling has already run and there's nothing left
	// to race against here before starting fresh.
	return s.Start(ctx, name)
}

// Status returns the current state of one service.
func (s *Supervisor) Status(name string) (State, error) {
	p, err := s.get(name)
	if err != nil {
		return State{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	st := State{
		Name:                p.spec.Name,
		Running:             p.running,
		Restarts:            p.restarts,
		LastExitCode:        p.lastExitCode,
		LastExitAt:          p.lastExitAt,
		GaveUpAfterRestarts: p.gaveUp,
	}
	if p.running {
		st.StartedAt = p.startedAt
		if p.cmd != nil && p.cmd.Process != nil {
			st.PID = p.cmd.Process.Pid
		}
	}
	return st, nil
}

// StatusAll returns Status for every registered service.
func (s *Supervisor) StatusAll() []State {
	s.mu.Lock()
	names := make([]string, 0, len(s.procs))
	for name := range s.procs {
		names = append(names, name)
	}
	s.mu.Unlock()

	out := make([]State, 0, len(names))
	for _, name := range names {
		if st, err := s.Status(name); err == nil {
			out = append(out, st)
		}
	}
	return out
}

// HealthCheck reports whether a service is healthy: if it has a HealthURL,
// that's polled (2xx = healthy); otherwise health is just "is the process
// running".
func (s *Supervisor) HealthCheck(ctx context.Context, name string) (Health, error) {
	p, err := s.get(name)
	if err != nil {
		return Health{}, err
	}
	p.mu.Lock()
	spec := p.spec
	running := p.running
	p.mu.Unlock()

	if !running {
		return Health{Name: name, Healthy: false, Detail: "process not running"}, nil
	}
	if spec.HealthURL == "" {
		return Health{Name: name, Healthy: true, Detail: "process running (no health endpoint configured)"}, nil
	}

	hctx, cancel := context.WithTimeout(ctx, spec.HealthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, spec.HealthURL, nil)
	if err != nil {
		return Health{Name: name, Healthy: false, Detail: err.Error()}, nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Health{Name: name, Healthy: false, Detail: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Health{Name: name, Healthy: true}, nil
	}
	return Health{Name: name, Healthy: false, Detail: fmt.Sprintf("health endpoint returned %d", resp.StatusCode)}, nil
}

func (s *Supervisor) serviceLogWriter(name string) (io.Writer, error) {
	if s.LogDir == "" {
		return os.Stdout, nil
	}
	if err := os.MkdirAll(s.LogDir, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(s.LogDir, name+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return f, nil
}

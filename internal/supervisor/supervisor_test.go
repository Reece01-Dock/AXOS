package supervisor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitFor polls cond every 20ms until it returns true or timeout elapses,
// failing the test if it times out. Keeps process-supervision tests (which
// are inherently a bit timing-dependent) from being flakier or slower than
// they need to be with fixed sleeps.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, what)
}

func TestStartStop_Basic(t *testing.T) {
	s := New("")
	s.Register(ServiceSpec{Name: "sleeper", Command: "sh", Args: []string{"-c", "sleep 5"}})

	if err := s.Start(context.Background(), "sleeper"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, 2*time.Second, "sleeper running", func() bool {
		st, _ := s.Status("sleeper")
		return st.Running && st.PID > 0
	})

	if err := s.Stop(context.Background(), "sleeper"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st, _ := s.Status("sleeper")
	if st.Running {
		t.Fatal("service still reports running after Stop")
	}

	// Give the (now-stopped) watcher time to have possibly auto-restarted
	// — it shouldn't have, since Stop marks stopRequested.
	time.Sleep(200 * time.Millisecond)
	st, _ = s.Status("sleeper")
	if st.Running {
		t.Fatal("Stop should prevent auto-restart, but the service is running again")
	}
	if st.Restarts != 0 {
		t.Errorf("Restarts = %d, want 0 (a deliberate Stop is not a crash)", st.Restarts)
	}
}

func TestCrashRestart(t *testing.T) {
	s := New("")
	s.Register(ServiceSpec{
		Name:                   "crasher",
		Command:                "sh",
		Args:                   []string{"-c", "exit 3"},
		RestartBackoffBase:     30 * time.Millisecond,
		RestartBackoffMax:      100 * time.Millisecond,
		MaxConsecutiveRestarts: 10,
	})

	if err := s.Start(context.Background(), "crasher"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, 3*time.Second, "at least 3 restarts", func() bool {
		st, _ := s.Status("crasher")
		return st.Restarts >= 3
	})

	st, _ := s.Status("crasher")
	if st.LastExitCode != 3 {
		t.Errorf("LastExitCode = %d, want 3", st.LastExitCode)
	}

	if err := s.Stop(context.Background(), "crasher"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestGivesUpAfterMaxConsecutiveRestarts(t *testing.T) {
	s := New("")
	s.Register(ServiceSpec{
		Name:                   "always-crashes",
		Command:                "sh",
		Args:                   []string{"-c", "exit 1"},
		RestartBackoffBase:     20 * time.Millisecond,
		RestartBackoffMax:      50 * time.Millisecond,
		MaxConsecutiveRestarts: 2,
	})

	if err := s.Start(context.Background(), "always-crashes"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, 3*time.Second, "supervisor gives up after max restarts", func() bool {
		st, _ := s.Status("always-crashes")
		return st.GaveUpAfterRestarts
	})

	// Restarts should stabilize once given up — no more attempts.
	st1, _ := s.Status("always-crashes")
	time.Sleep(300 * time.Millisecond)
	st2, _ := s.Status("always-crashes")
	if st2.Restarts != st1.Restarts {
		t.Errorf("Restarts kept climbing after giving up: %d -> %d", st1.Restarts, st2.Restarts)
	}
	if st2.Running {
		t.Error("a given-up service should not be running")
	}
}

func TestRestart_GetsANewPID(t *testing.T) {
	s := New("")
	s.Register(ServiceSpec{Name: "sleeper", Command: "sh", Args: []string{"-c", "sleep 5"}})

	if err := s.Start(context.Background(), "sleeper"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 2*time.Second, "initial start", func() bool {
		st, _ := s.Status("sleeper")
		return st.Running
	})
	st1, _ := s.Status("sleeper")

	if err := s.Restart(context.Background(), "sleeper"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	waitFor(t, 2*time.Second, "restart completes", func() bool {
		st, _ := s.Status("sleeper")
		return st.Running
	})
	st2, _ := s.Status("sleeper")

	if st2.PID == st1.PID {
		t.Errorf("PID unchanged after Restart (%d) — expected a new process", st1.PID)
	}

	s.Stop(context.Background(), "sleeper")
}

func TestHealthCheck_HTTPEndpoint(t *testing.T) {
	healthy := true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	s := New("")
	s.Register(ServiceSpec{
		Name: "sleeper", Command: "sh", Args: []string{"-c", "sleep 5"},
		HealthURL: ts.URL, HealthTimeout: time.Second,
	})
	if err := s.Start(context.Background(), "sleeper"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop(context.Background(), "sleeper")

	waitFor(t, 2*time.Second, "process running", func() bool {
		st, _ := s.Status("sleeper")
		return st.Running
	})

	h, err := s.HealthCheck(context.Background(), "sleeper")
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !h.Healthy {
		t.Errorf("HealthCheck = %+v, want Healthy=true", h)
	}

	healthy = false
	h, err = s.HealthCheck(context.Background(), "sleeper")
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if h.Healthy {
		t.Errorf("HealthCheck = %+v, want Healthy=false once the endpoint returns 500", h)
	}
}

func TestHealthCheck_NoURLFallsBackToProcessLiveness(t *testing.T) {
	s := New("")
	s.Register(ServiceSpec{Name: "sleeper", Command: "sh", Args: []string{"-c", "sleep 5"}})

	if err := s.Start(context.Background(), "sleeper"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 2*time.Second, "process running", func() bool {
		st, _ := s.Status("sleeper")
		return st.Running
	})

	h, err := s.HealthCheck(context.Background(), "sleeper")
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !h.Healthy {
		t.Errorf("HealthCheck with no HealthURL and a running process should report healthy, got %+v", h)
	}

	if err := s.Stop(context.Background(), "sleeper"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	h, err = s.HealthCheck(context.Background(), "sleeper")
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if h.Healthy {
		t.Error("HealthCheck after Stop should report unhealthy")
	}
}

func TestStatusAll(t *testing.T) {
	s := New("")
	s.Register(ServiceSpec{Name: "a", Command: "sh", Args: []string{"-c", "sleep 5"}})
	s.Register(ServiceSpec{Name: "b", Command: "sh", Args: []string{"-c", "sleep 5"}})

	if err := s.Start(context.Background(), "a"); err != nil {
		t.Fatalf("Start a: %v", err)
	}
	defer s.Stop(context.Background(), "a")

	waitFor(t, 2*time.Second, "a running", func() bool {
		st, _ := s.Status("a")
		return st.Running
	})

	all := s.StatusAll()
	if len(all) != 2 {
		t.Fatalf("StatusAll() has %d entries, want 2", len(all))
	}
	byName := map[string]State{}
	for _, st := range all {
		byName[st.Name] = st
	}
	if !byName["a"].Running {
		t.Error("service a should be running")
	}
	if byName["b"].Running {
		t.Error("service b should not be running")
	}
}

func TestUnknownService(t *testing.T) {
	s := New("")
	if err := s.Start(context.Background(), "nope"); err == nil {
		t.Error("Start on unknown service should fail")
	}
	if err := s.Stop(context.Background(), "nope"); err == nil {
		t.Error("Stop on unknown service should fail")
	}
	if _, err := s.Status("nope"); err == nil {
		t.Error("Status on unknown service should fail")
	}
}

func TestLogDir_CapturesServiceOutput(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Register(ServiceSpec{Name: "echoer", Command: "sh", Args: []string{"-c", "echo hello-from-service"}})

	if err := s.Start(context.Background(), "echoer"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, 2*time.Second, "log file has content", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, "echoer.log"))
		return err == nil && len(data) > 0
	})

	data, err := os.ReadFile(filepath.Join(dir, "echoer.log"))
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if !strings.Contains(string(data), "hello-from-service") {
		t.Errorf("log content = %q, want it to contain %q", data, "hello-from-service")
	}

	s.Stop(context.Background(), "echoer")
}

package svcconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_MissingFileReturnsEmptyNotError(t *testing.T) {
	specs, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load of a missing file should not error, got: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("Load of a missing file = %v, want empty", specs)
	}
}

func TestLoad_EmptyPathReturnsEmptyNotError(t *testing.T) {
	specs, err := Load("")
	if err != nil || len(specs) != 0 {
		t.Fatalf("Load(\"\") = %v, %v; want (nil, nil)", specs, err)
	}
}

func TestLoad_ParsesEntriesAndDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	content := `[
		{
			"name": "axos-monitor",
			"command": "/opt/axos/current/bin/axos-monitor",
			"args": ["-api", "http://127.0.0.1:9090"],
			"health_url": "http://127.0.0.1:9191/healthz",
			"restart_backoff_base": "2s",
			"restart_backoff_max": "1m",
			"max_consecutive_restarts": 3
		}
	]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	specs, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("Load returned %d specs, want 1", len(specs))
	}
	s := specs[0]
	if s.Name != "axos-monitor" || s.Command != "/opt/axos/current/bin/axos-monitor" {
		t.Errorf("spec = %+v", s)
	}
	if s.RestartBackoffBase != 2*time.Second {
		t.Errorf("RestartBackoffBase = %v, want 2s", s.RestartBackoffBase)
	}
	if s.RestartBackoffMax != time.Minute {
		t.Errorf("RestartBackoffMax = %v, want 1m", s.RestartBackoffMax)
	}
	if s.MaxConsecutiveRestarts != 3 {
		t.Errorf("MaxConsecutiveRestarts = %d, want 3", s.MaxConsecutiveRestarts)
	}
}

func TestLoad_RejectsEntryMissingRequiredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, []byte(`[{"name":"no-command"}]`), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load should reject an entry missing \"command\"")
	}
}

func TestLoad_RejectsInvalidDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	content := `[{"name":"x","command":"/bin/x","restart_backoff_base":"not-a-duration"}]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load should reject an invalid duration string")
	}
}

func TestLoad_RejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, []byte(`not json`), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load should reject malformed JSON")
	}
}

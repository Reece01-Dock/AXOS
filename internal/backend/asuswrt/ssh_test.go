package asuswrt

import (
	"strings"
	"testing"
)

// These test the SSH argv-construction logic in isolation — no network
// connection is attempted anywhere in this file. Actually connecting to a
// real router over SSH is untested in this environment (no reachable
// GT-AX6000); this proves the command gets built correctly, which is the
// part that's safe and meaningful to verify without hardware.

func TestShellQuote_HandlesSpacesAndEmbeddedQuotes(t *testing.T) {
	cases := map[string]string{
		"":                 "''",
		"simple":           "'simple'",
		"has spaces":       "'has spaces'",
		`it's got a quote`: `'it'\''s got a quote'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildSSHArgv_Structure(t *testing.T) {
	argv := buildSSHArgv("ssh", []string{"-o", "BatchMode=yes"}, "router.local", "nvram", []string{"get", "productid"})

	want := []string{"ssh", "-o", "BatchMode=yes", "router.local", "'nvram' 'get' 'productid'"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %#v, want length %d", argv, len(want))
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func TestBuildSSHArgv_QuotesArgumentsWithSpaces(t *testing.T) {
	argv := buildSSHArgv("ssh", nil, "router", "sh", []string{"-c", "echo hello world"})
	remoteCmd := argv[len(argv)-1]
	if !strings.Contains(remoteCmd, "'echo hello world'") {
		t.Errorf("remote command = %q, want the multi-word arg quoted as a single shell token", remoteCmd)
	}
}

func TestNewSSHRunner_AppliesDefaultsThenExtraArgs(t *testing.T) {
	r := newSSHRunner("router", "-p", "2222")

	if r.host != "router" {
		t.Errorf("host = %q, want router", r.host)
	}
	joined := strings.Join(r.extraArgs, " ")
	if !strings.Contains(joined, "BatchMode=yes") {
		t.Error("newSSHRunner should enforce BatchMode=yes by default (key-only auth, per docs/security.md)")
	}
	if !strings.Contains(joined, "-p 2222") {
		t.Error("extra args passed to newSSHRunner should be appended and preserved")
	}
}

func TestWithHost_SwitchesBackendToSSHRunner(t *testing.T) {
	b, err := New(WithHost("router.example"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if b.Host != "router.example" {
		t.Errorf("b.Host = %q, want router.example", b.Host)
	}
	if _, ok := b.runner.(*sshRunner); !ok {
		t.Errorf("b.runner = %T, want *sshRunner after WithHost", b.runner)
	}
}

func TestNew_DefaultsToLocalExecRunner(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := b.runner.(execRunner); !ok {
		t.Errorf("b.runner = %T, want execRunner by default (no WithHost)", b.runner)
	}
	if b.Host != "" {
		t.Errorf("b.Host = %q, want empty by default", b.Host)
	}
}

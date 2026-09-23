// Package pkgmod is a Milestone "later" skeleton for optional AXOS packages.
//
// Packages are hot-deployable optional features (never baked into squashfs by
// default). This package only defines the registry contract and an in-memory
// stub — install/enable against USB storage is not implemented yet.
package pkgmod

import (
	"fmt"
	"sync"
)

// Info describes one optional package.
type Info struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// Registry tracks known packages. Stub implementations do not touch disk.
type Registry interface {
	List() []Info
	Get(id string) (Info, bool)
	Enable(id string) error
	Disable(id string) error
}

// Memory is an in-memory Registry for tests and early bring-up.
type Memory struct {
	mu   sync.Mutex
	pkgs map[string]Info
}

// NewMemory returns a registry seeded with no packages.
func NewMemory() *Memory {
	return &Memory{pkgs: map[string]Info{}}
}

// Seed adds or replaces package metadata (enabled=false).
func (m *Memory) Seed(info Info) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info.Enabled = false
	m.pkgs[info.ID] = info
}

func (m *Memory) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.pkgs))
	for _, p := range m.pkgs {
		out = append(out, p)
	}
	return out
}

func (m *Memory) Get(id string) (Info, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pkgs[id]
	return p, ok
}

func (m *Memory) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pkgs[id]
	if !ok {
		return fmt.Errorf("pkgmod: unknown package %q", id)
	}
	p.Enabled = true
	m.pkgs[id] = p
	return nil
}

func (m *Memory) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pkgs[id]
	if !ok {
		return fmt.Errorf("pkgmod: unknown package %q", id)
	}
	p.Enabled = false
	m.pkgs[id] = p
	return nil
}

var _ Registry = (*Memory)(nil)

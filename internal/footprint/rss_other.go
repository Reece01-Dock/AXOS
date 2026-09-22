//go:build !linux

package footprint

import "fmt"

// AXOS only ever runs axosd on Linux (the router, or a Linux dev machine) —
// this stub keeps `go build/test ./...` working on a contributor's Mac or
// Windows machine. Measure() treats this error as non-fatal and leaves
// ProcessRSSKB at 0.
func readProcessRSSKB() (uint64, error) {
	return 0, fmt.Errorf("footprint: process RSS measurement is only implemented on linux")
}

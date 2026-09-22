//go:build linux

package footprint

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// readProcessRSSKB reads the calling process's own resident set size from
// /proc/self/status. This is the number that matters for "how much RAM does
// AXOS cost" on the router — it's real physical memory in use, not the
// Go heap's notion of allocated-but-possibly-unmapped memory.
func readProcessRSSKB() (uint64, error) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		// Expected form: "VmRSS:	  1234 kB" — fields[1] is the value,
		// already in kB per the kernel's documented /proc/PID/status format.
		if len(fields) < 2 {
			return 0, fmt.Errorf("footprint: unexpected VmRSS line %q", line)
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("footprint: parsing VmRSS line %q: %w", line, err)
		}
		return v, nil
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("footprint: VmRSS not found in /proc/self/status")
}

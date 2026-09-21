package main

import (
	"context"
	"flag"
)

// cmdTest runs this repo's Go test suite with output streamed straight to
// the terminal — the "TEST LOCALLY" step of the hot-deploy loop, callable
// on its own or via "axosctl dev test".
func cmdTest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	_ = fs.Parse(args)
	return runGoTest()
}

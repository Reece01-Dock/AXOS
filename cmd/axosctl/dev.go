package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reece01-dock/axos/internal/backend/httpclient"
)

func cmdDev(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: axosctl dev <deploy|watch|restart|test> [flags]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "deploy":
		return cmdDeploy(ctx, rest)
	case "restart":
		return cmdRestart(ctx, rest)
	case "test":
		return cmdTest(ctx, rest)
	case "watch":
		return cmdDevWatch(ctx, rest)
	default:
		return fmt.Errorf("unknown dev subcommand %q (want deploy, watch, restart, or test)", sub)
	}
}

// cmdDevWatch is the flagship hot-development loop:
//
//	file changes -> test -> build -> stage -> promote -> restart -> health check
//
// repeating on every change, skipping deploy entirely on a failed test (so
// staging/current is never touched by a build that didn't pass), and never
// touching the router's firmware or requiring a reboot for any of it.
//
// Change detection is a simple poll (max mtime + file count across *.go
// files under --watch-dir) rather than an inotify/fsnotify-based watch —
// a deliberate simplicity choice to avoid a new dependency; --interval
// controls the poll period.
func cmdDevWatch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dev watch", flag.ExitOnError)
	watchDir := fs.String("watch-dir", ".", "Directory tree to watch for *.go changes")
	services := fs.String("service", "", "Comma-separated supervised service(s) to restart after a successful deploy (optional)")
	interval := fs.Duration("interval", 2*time.Second, "Poll interval for change detection")
	f := addDeployFlags(fs)
	apiURL, actor := addAPIFlags(fs)
	_ = fs.Parse(args)

	client := newAPIClient(apiURL, actor)

	fmt.Printf("axosctl dev watch: watching %s (poll every %s). Ctrl-C to stop.\n", *watchDir, *interval)

	baseline, err := fingerprint(*watchDir)
	if err != nil {
		return fmt.Errorf("watching %s: %w", *watchDir, err)
	}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\naxosctl dev watch: stopping.")
			return nil
		case <-ticker.C:
			current, err := fingerprint(*watchDir)
			if err != nil {
				fmt.Printf("watch: %v\n", err)
				continue
			}
			if current == baseline {
				continue
			}
			baseline = current // debounce: don't re-trigger on this same state again

			fmt.Println("\n==> Change detected")
			if err := runDeploy(ctx, f); err != nil {
				fmt.Printf("==> Deploy FAILED, not restarting anything: %v\n", err)
				continue
			}

			if *services == "" {
				fmt.Println("==> Deployed. No --service given, so nothing was restarted.")
				continue
			}
			restartAndHealthCheck(ctx, client, strings.Split(*services, ","))
		}
	}
}

// restartAndHealthCheck restarts each named service and reports its health
// a moment later. It never returns an error: a restart/health failure here
// means "watch, print, keep watching", not "stop the loop" — the whole
// point of dev watch is that it survives a bad iteration and keeps going.
func restartAndHealthCheck(ctx context.Context, client *httpclient.Client, names []string) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if err := client.ServiceRestart(ctx, name); err != nil {
			fmt.Printf("==> %s: restart FAILED: %v\n", name, err)
			continue
		}
		time.Sleep(300 * time.Millisecond) // let it come up before checking
		h, err := client.ServiceHealth(ctx, name)
		switch {
		case err != nil:
			fmt.Printf("==> %s: restarted, health check FAILED: %v\n", name, err)
		case !h.Healthy:
			fmt.Printf("==> %s: restarted, UNHEALTHY: %s\n", name, h.Detail)
		default:
			fmt.Printf("==> %s: restarted, healthy\n", name)
		}
	}
}

func fingerprint(dir string) (string, error) {
	var maxMod time.Time
	var count int
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Don't descend into the release root or VCS metadata if
			// they happen to be under watch-dir — neither is source.
			base := filepath.Base(path)
			if base == ".git" || base == "releases" || base == "staging" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		count++
		if info.ModTime().After(maxMod) {
			maxMod = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", count, maxMod.UnixNano()), nil
}

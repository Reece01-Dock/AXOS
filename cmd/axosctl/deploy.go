package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/reece01-dock/axos/internal/deploy"
)

var defaultComponents = []string{"axosd", "axos-mcp", "axosctl"}

type deployFlags struct {
	root       *string
	components *string
	goos       *string
	goarch     *string
	skipTests  *bool
	skipBuild  *bool
}

func addDeployFlags(fs *flag.FlagSet) deployFlags {
	return deployFlags{
		root:       rootFlag(fs),
		components: fs.String("components", strings.Join(defaultComponents, ","), "Comma-separated cmd/ directories to build"),
		goos:       fs.String("goos", "", "Cross-compile target OS (default: host)"),
		goarch:     fs.String("goarch", "", "Cross-compile target arch (e.g. arm64 for the router; default: host)"),
		skipTests:  fs.Bool("skip-tests", false, "Skip running the test suite before deploying"),
		skipBuild:  fs.Bool("skip-build", false, "Skip the build step (deploy whatever's already in staging)"),
	}
}

// cmdDeploy is the build->test->stage->promote pipeline: "EDIT -> BUILD ->
// TEST LOCALLY -> DEPLOY" from the top of docs/development.md, minus the
// SSH-to-a-real-router step (which is scripts/deploy-router.sh's job —
// this command promotes into --root, which is /opt/axos when run ON the
// router, or a local directory for dev-loop testing against mock).
func cmdDeploy(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	f := addDeployFlags(fs)
	_ = fs.Parse(args)

	return runDeploy(ctx, f)
}

func runDeploy(ctx context.Context, f deployFlags) error {
	if !*f.skipTests {
		fmt.Println("==> Running tests")
		if err := runGoTest(); err != nil {
			return fmt.Errorf("tests failed, not deploying: %w", err)
		}
	}

	d := deploy.New(*f.root)
	components := strings.Split(*f.components, ",")

	if !*f.skipBuild {
		fmt.Printf("==> Building %s\n", strings.Join(components, ", "))
		if err := buildComponents(d.StagingDir(), components, *f.goos, *f.goarch); err != nil {
			return fmt.Errorf("build failed, not deploying: %w", err)
		}
	}

	commit := gitCommit()
	manifest, err := deploy.BuildManifest(d.StagingDir(), commit, components)
	if err != nil {
		return fmt.Errorf("building manifest: %w", err)
	}

	fmt.Println("==> Promoting staged build to a new release")
	res, err := d.Deploy(ctx, manifest)
	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	fmt.Printf("Deployed release %s (commit %s)\n", res.ReleaseID, commit)
	if res.PreviousReleaseID != "" {
		fmt.Printf("Previous release %s is still available: axosctl rollback\n", res.PreviousReleaseID)
	}
	fmt.Printf("\nNothing has been restarted yet — run:\n  axosctl restart <service>\nfor each affected supervised service, then check:\n  axosctl health\n")
	return nil
}

// buildComponents cross-compiles each `cmd/<name>` package into
// outDir/bin/<name>. goos/goarch empty means "host" (no cross-compile env
// vars set, i.e. whatever `go build` does by default).
func buildComponents(outDir string, components []string, goos, goarch string) error {
	binDir := filepath.Join(outDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}
	for _, name := range components {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		pkg := "./cmd/" + name
		out := filepath.Join(binDir, name)
		fmt.Printf("    go build -o %s %s\n", out, pkg)

		cmd := exec.Command("go", "build", "-o", out, pkg)
		cmd.Env = os.Environ()
		if goos != "" {
			cmd.Env = append(cmd.Env, "GOOS="+goos)
		}
		if goarch != "" {
			cmd.Env = append(cmd.Env, "GOARCH="+goarch)
		}
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("building %s: %w", pkg, err)
		}
	}
	return nil
}

func runGoTest() error {
	// Only AXOS packages — never ./..., which would pick up stray Go under
	// firmware/src-stock (local Merlin clone) when that tree is present.
	// Nested go.mod files under firmware/src{,-stock}/ are a second line of
	// defence; this keep-list is the authoritative one for deploy.
	cmd := exec.Command("go", "test", "./cmd/...", "./internal/...")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// gitCommit returns the short commit hash of HEAD, or "unknown" if this
// isn't a git checkout or git isn't available — never fabricated.
func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func cmdRollback(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	root := rootFlag(fs)
	version := fs.String("version", "", "Release id to roll back to (default: the previous release)")
	_ = fs.Parse(args)

	d := deploy.New(*root)
	res, err := d.Rollback(ctx, *version)
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	fmt.Printf("Rolled back to release %s\n", res.ReleaseID)
	fmt.Println("Restart affected services to pick it up: axosctl restart <service>")
	return nil
}

func cmdHistory(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	root := rootFlag(fs)
	_ = fs.Parse(args)

	d := deploy.New(*root)
	hist, err := d.History(ctx)
	if err != nil {
		return fmt.Errorf("reading history: %w", err)
	}
	if len(hist) == 0 {
		fmt.Println("No releases yet.")
		return nil
	}
	for _, r := range hist {
		marker := "  "
		if r.IsCurrent {
			marker = "->"
		} else if r.IsPrevious {
			marker = " *"
		}
		commit := "unknown"
		components := ""
		if r.Manifest != nil {
			commit = r.Manifest.Commit
			components = strings.Join(r.Manifest.Components, ",")
		}
		fmt.Printf("%s %s  commit=%-10s  %s  %s\n", marker, r.ID, commit, r.CreatedAt.Format("2006-01-02 15:04:05"), components)
	}
	fmt.Println("\n-> current   * previous")
	return nil
}

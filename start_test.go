package main

import (
	"strings"
	"testing"

	"github.com/nclandrei/proctor/internal/proctor"
)

func TestStartErrorsOnMissingFlagsInsteadOfPrompting(t *testing.T) {
	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	initGitRepoForCLI(t, repoRoot, "https://github.com/nclandrei/proctor-test-prompts")

	withWorkingDirectory(t, repoRoot, func() {
		// Only pass --platform, omit all other required flags.
		err := run([]string{"start", "--platform", "web"})
		if err == nil {
			t.Fatal("expected error for missing required flags, got nil")
		}
		msg := err.Error()
		// Should NOT be an EOF error from interactive prompts.
		if strings.Contains(msg, "EOF") {
			t.Fatalf("got interactive prompt EOF error instead of listing missing flags: %s", msg)
		}
		// Should mention the missing flags.
		if !strings.Contains(msg, "--feature") {
			t.Fatalf("expected error to mention --feature, got: %s", msg)
		}
		if !strings.Contains(msg, "--url") {
			t.Fatalf("expected error to mention --url, got: %s", msg)
		}
		if !strings.Contains(msg, "--happy-path") {
			t.Fatalf("expected error to mention --happy-path, got: %s", msg)
		}
		if !strings.Contains(msg, "--failure-path") {
			t.Fatalf("expected error to mention --failure-path, got: %s", msg)
		}
	})
}

func TestStartRejectsWhenActiveRunExists(t *testing.T) {
	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	initGitRepoForCLI(t, repoRoot, "https://github.com/nclandrei/proctor-test-active")

	startArgs := fullWebStartArgs()

	withWorkingDirectory(t, repoRoot, func() {
		// First start should succeed.
		if err := run(startArgs); err != nil {
			t.Fatalf("first start failed: %v", err)
		}

		// Second start without --force should fail.
		err := run(startArgs)
		if err == nil {
			t.Fatal("expected error when active run already exists, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, "active run already exists") {
			t.Fatalf("expected 'active run already exists' error, got: %s", msg)
		}

		// Second start WITH --force should succeed.
		forceArgs := append(startArgs, "--force")
		if err := run(forceArgs); err != nil {
			t.Fatalf("start with --force failed: %v", err)
		}
	})
}

func fullWebStartArgs() []string {
	args := []string{
		"start",
		"--platform", "web",
		"--feature", "test feature",
		"--url", "http://localhost:3000",
		"--happy-path", "page loads",
		"--failure-path", "page errors",
		"--curl", "skip",
		"--curl-skip-reason", "no backend",
	}
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformWeb) {
		args = append(args, "--edge-case", category+"=N/A: test coverage")
	}
	return args
}

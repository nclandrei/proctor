package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDetectCaptureToolsWithLookPath(t *testing.T) {
	tools := detectCaptureToolsWithLookPath(func(name string) (string, error) {
		switch name {
		case "agent-browser", "ghostty", "tmux", "curl":
			return "/usr/bin/" + name, nil
		default:
			return "", fmt.Errorf("missing %s", name)
		}
	})

	if !tools.AgentBrowser || !tools.Ghostty || !tools.Tmux || !tools.Curl {
		t.Fatalf("expected selected tools to be detected, got %#v", tools)
	}
	if tools.Xcrun || tools.Script {
		t.Fatalf("expected absent tools to be false, got %#v", tools)
	}
}

func TestRunStartPrintsRecommendedWorkflowForWebRun(t *testing.T) {
	withStubbedLookPath(t, "agent-browser", "curl")

	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	initGitRepoForCLI(t, repoRoot, "https://github.com/nclandrei/proctor-start-test")

	withWorkingDirectory(t, repoRoot, func() {
		output := captureStdout(t, func() {
			if err := run([]string{
				"start",
				"--platform", "web",
				"--feature", "auth flow",
				"--url", "http://127.0.0.1:3000/login",
				"--curl", "scenario",
				"--curl-endpoint", "happy-path=POST /api/login",
				"--happy-path", "valid login goes to dashboard",
				"--failure-path", "invalid password shows error",
				"--edge-case", "validation and malformed input=Bad email shows inline validation",
				"--edge-case", "empty or missing input=Empty email and password show required-field errors",
				"--edge-case", "retry or double-submit=Second submit does not create duplicate requests",
				"--edge-case", "loading, latency, and race conditions=Button stays disabled while the request is pending",
				"--edge-case", "network or server failure=500 response shows a retryable error state",
				"--edge-case", "auth and session state=Already signed-in users are redirected away from /login",
				"--edge-case", "refresh, back-navigation, and state persistence=Refresh preserves the authenticated state",
				"--edge-case", "mobile or responsive behavior=Login form stays usable at mobile width",
				"--edge-case", "accessibility and keyboard behavior=Enter submits from the password field; tab order stays correct",
				"--edge-case", "any feature-specific risks=N/A: no extra feature-specific risks",
			}); err != nil {
				t.Fatal(err)
			}
		})

		for _, needle := range []string{
			"Recommended next step:",
			"`agent-browser` detected on PATH",
			"`curl` is detected on PATH",
		} {
			if !strings.Contains(output, needle) {
				t.Fatalf("expected start output to include %q, got:\n%s", needle, output)
			}
		}
	})
}

func TestRunStatusPrintsSuggestedWorkflowForIncompleteCLIRun(t *testing.T) {
	withStubbedLookPath(t, "ghostty", "tmux")

	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	initGitRepoForCLI(t, repoRoot, "https://github.com/nclandrei/proctor-cli-status-test")

	withWorkingDirectory(t, repoRoot, func() {
		if err := run([]string{
			"start",
			"--platform", "cli",
			"--feature", "cli inspection flow",
			"--cli-command", "demo help",
			"--happy-path", "help output is readable",
			"--failure-path", "unknown argument fails clearly",
			"--edge-case", "invalid or malformed input=Broken prompt syntax shows a validation error without a panic",
			"--edge-case", "missing required args, files, config, or env=Missing prompt slug explains what argument is required",
			"--edge-case", "retry, rerun, and idempotency=Running the same inspect command twice gives the same result",
			"--edge-case", "long-running output, streaming, or progress state=N/A: single-shot command with immediate output",
			"--edge-case", "interrupts, cancellation, and signals=N/A: command exits immediately",
			"--edge-case", "tty, pipe, and non-interactive behavior=Piped output still renders the inspected prompt body without ANSI garbage",
			"--edge-case", "terminal layout, wrapping, and resize behavior=The inspected prompt still wraps cleanly in a narrow terminal",
			"--edge-case", "keyboard navigation and shortcut behavior=N/A: single-shot command with no in-app key handling",
			"--edge-case", "state, config, and persistence across reruns=N/A: read-only inspection command",
			"--edge-case", "stderr, exit codes, and partial failure reporting=Unknown prompt returns a non-zero exit code and prints the error on stderr",
			"--edge-case", "any feature-specific risks=N/A: no extra feature-specific risks",
		}); err != nil {
			t.Fatal(err)
		}

		output := captureStdout(t, func() {
			if err := run([]string{"status"}); err != nil {
				t.Fatal(err)
			}
		})

		for _, needle := range []string{
			"Status: incomplete",
			"Suggested capture workflows:",
			"`ghostty` and `tmux` are detected on PATH",
		} {
			if !strings.Contains(output, needle) {
				t.Fatalf("expected status output to include %q, got:\n%s", needle, output)
			}
		}
	})
}

func TestRunDonePrintsSuggestedWorkflowForIncompleteIOSRun(t *testing.T) {
	withStubbedLookPath(t, "xcrun")

	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	initGitRepoForCLI(t, repoRoot, "https://github.com/nclandrei/proctor-ios-done-test")

	withWorkingDirectory(t, repoRoot, func() {
		if err := run([]string{
			"start",
			"--platform", "ios",
			"--feature", "library flow",
			"--ios-scheme", "Pagena",
			"--ios-bundle-id", "com.example.pagena",
			"--ios-simulator", "iPhone 16 Pro",
			"--curl", "skip",
			"--curl-skip-reason", "UI-only verification",
			"--happy-path", "launching the app lands on the library screen",
			"--failure-path", "an unavailable chapter shows a visible fallback state",
			"--edge-case", "validation and malformed input=N/A: no freeform input in this flow",
			"--edge-case", "empty or missing input=N/A: no required input in this flow",
			"--edge-case", "retry or double-submit=N/A: no repeated mutation in this flow",
			"--edge-case", "loading, latency, and race conditions=N/A: no async mutation in this flow",
			"--edge-case", "network or server failure=N/A: no backend dependency in this flow",
			"--edge-case", "auth and session state=N/A: anonymous browsing only",
			"--edge-case", "app lifecycle, relaunch, and state persistence=N/A: covered elsewhere",
			"--edge-case", "device traits, orientation, and layout=N/A: covered elsewhere",
			"--edge-case", "accessibility, dynamic type, and keyboard behavior=N/A: visual pass only",
			"--edge-case", "any feature-specific risks=N/A: no additional risks",
		}); err != nil {
			t.Fatal(err)
		}

		var runErr error
		output := captureStdout(t, func() {
			runErr = run([]string{"done"})
		})
		if runErr == nil {
			t.Fatal("expected done to fail without evidence")
		}

		for _, needle := range []string{
			"FAIL",
			"Suggested capture workflows:",
			"`xcrun` detected on PATH",
		} {
			if !strings.Contains(output, needle) {
				t.Fatalf("expected done output to include %q, got:\n%s", needle, output)
			}
		}
	})
}

func withStubbedLookPath(t *testing.T, present ...string) {
	t.Helper()

	lookup := map[string]bool{}
	for _, name := range present {
		lookup[name] = true
	}

	old := lookPathFn
	lookPathFn = func(name string) (string, error) {
		if lookup[name] {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() {
		lookPathFn = old
	})
}

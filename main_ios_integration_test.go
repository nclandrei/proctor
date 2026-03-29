package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIOSFlowViaCLI(t *testing.T) {
	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	initGitRepoForCLI(t, repoRoot, "https://github.com/nclandrei/pagena")

	reportPath := writeCLIFile(t, repoRoot, "ios-report.json", `{
  "simulator": {
    "name": "iPhone 16 Pro",
    "runtime": "iOS 18.2"
  },
  "app": {
    "bundleId": "com.example.pagena",
    "screen": "Library",
    "state": "foreground"
  },
  "issues": {
    "launchErrors": 0,
    "crashes": 0,
    "fatalLogs": 0
  }
}`)
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

		for i, scenarioID := range []string{"happy-path", "failure-path"} {
			screenshotPath := writeCLIScreenshot(t, repoRoot, fmt.Sprintf("library-%d.png", i), fmt.Sprintf("image-%s", scenarioID))
			if err := run([]string{
				"record", "ios",
				"--scenario", scenarioID,
				"--session", "pagena-library-1",
				"--report", reportPath,
				"--screenshot", "library=" + screenshotPath,
				"--assert", "screen contains Library",
				"--assert", "bundle_id = com.example.pagena",
				"--assert", "app_launch = true",
			}); err != nil {
				t.Fatal(err)
			}
		}

		statusOutput := captureStdout(t, func() {
			if err := run([]string{"status"}); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(statusOutput, "ios: pass") {
			t.Fatalf("expected status to report ios coverage, got:\n%s", statusOutput)
		}

		if err := run([]string{"done"}); err != nil {
			t.Fatal(err)
		}

		reportOutput := captureStdout(t, func() {
			if err := run([]string{"report"}); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(reportOutput, "report.html") || !strings.Contains(reportOutput, "contract.md") {
			t.Fatalf("expected report command to print derived report paths, got:\n%s", reportOutput)
		}
	})
}

func initGitRepoForCLI(t *testing.T, repo, remote string) {
	t.Helper()
	mustRunCLI(t, repo, "git", "init")
	mustRunCLI(t, repo, "git", "config", "user.email", "test@example.com")
	mustRunCLI(t, repo, "git", "config", "user.name", "Test User")
	mustRunCLI(t, repo, "git", "remote", "add", "origin", remote)
}

func mustRunCLI(t *testing.T, dir string, command string, args ...string) {
	t.Helper()
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", command, args, err, string(out))
	}
}

func withWorkingDirectory(t *testing.T, dir string, fn func()) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	fn()
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		done <- buf.String()
	}()

	fn()

	_ = writer.Close()
	os.Stdout = oldStdout
	return <-done
}

func writeCLIFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeCLIScreenshot creates a file that exceeds the minimum screenshot size threshold.
func writeCLIScreenshot(t *testing.T, dir, name, content string) string {
	t.Helper()
	minSize := 10*1024 + 1
	padded := content
	for len(padded) < minSize {
		padded += "\x00"
	}
	return writeCLIFile(t, dir, name, padded)
}

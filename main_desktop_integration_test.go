package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestRunDesktopFlowViaCLI(t *testing.T) {
	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	initGitRepoForCLI(t, repoRoot, "https://github.com/nclandrei/proctor-desktop-test")

	reportPath := writeCLIFile(t, repoRoot, "desktop-report.json", `{
  "app": {
    "name": "Firefox",
    "bundleId": "org.mozilla.firefox",
    "state": "running",
    "windowTitle": "Bookmark Manager"
  },
  "issues": {
    "crashes": 0,
    "fatalLogs": 0
  }
}`)
	withWorkingDirectory(t, repoRoot, func() {
		if err := run([]string{
			"start",
			"--platform", "desktop",
			"--feature", "bookmark manager",
			"--app-name", "Firefox",
			"--app-bundle-id", "org.mozilla.firefox",
			"--curl", "skip",
			"--curl-skip-reason", "UI-only desktop verification",
			"--happy-path", "bookmark manager opens and lists saved bookmarks",
			"--failure-path", "empty bookmark list shows a helpful prompt",
			"--edge-case", "validation and malformed input=N/A: no freeform input in this flow",
			"--edge-case", "empty or missing input=N/A: no required input in this flow",
			"--edge-case", "retry or double-submit=N/A: no repeated mutation in this flow",
			"--edge-case", "loading, latency, and race conditions=N/A: instant local operation",
			"--edge-case", "network or server failure=N/A: no backend dependency in this flow",
			"--edge-case", "auth and session state=N/A: no authentication required",
			"--edge-case", "window management, resize, and multi-monitor=N/A: covered elsewhere",
			"--edge-case", "drag-drop, clipboard, and system integration=N/A: no drag-drop in this flow",
			"--edge-case", "keyboard shortcuts and accessibility=N/A: this pass is visual only",
			"--edge-case", "any feature-specific risks=N/A: no additional risks",
		}); err != nil {
			t.Fatal(err)
		}

		for i, scenarioID := range []string{"happy-path", "failure-path"} {
			screenshotPath := writeCLIScreenshot(t, repoRoot, fmt.Sprintf("window-%d.png", i), fmt.Sprintf("image-%s", scenarioID))
			if err := run([]string{
				"note",
				"--scenario", scenarioID,
				"--session", "firefox-desktop-1",
				"--notes", "about to verify Firefox bookmark manager window for scenario " + scenarioID,
			}); err != nil {
				t.Fatal(err)
			}
			if err := run([]string{
				"record", "desktop",
				"--scenario", scenarioID,
				"--session", "firefox-desktop-1",
				"--report", reportPath,
				"--screenshot", "window=" + screenshotPath,
				"--assert", "app_name contains Firefox",
				"--assert", "crashes = 0",
			}); err != nil {
				t.Fatal(err)
			}
		}

		for i, scenarioID := range []string{"happy-path", "failure-path"} {
			logShot := writeCLIScreenshot(t, repoRoot, fmt.Sprintf("log-%d.png", i), fmt.Sprintf("log-image-%s", scenarioID))
			if err := run([]string{
				"log",
				"--scenario", scenarioID,
				"--session", "firefox-desktop-1",
				"--surface", "desktop",
				"--screenshot", logShot,
				"--action", "opened Firefox bookmark manager and inspected the window for " + scenarioID,
				"--observation", "Firefox bookmark manager window visible with Bookmarks title and saved entries",
				"--comparison", "matches the " + scenarioID + " scenario requirements from the contract",
			}); err != nil {
				t.Fatal(err)
			}
			if err := run([]string{
				"verify",
				"--scenario", scenarioID,
				"--session", "firefox-desktop-1",
				"--notes", "Firefox bookmark manager window visible with Bookmarks title and saved entries list shown",
			}); err != nil {
				t.Fatal(err)
			}
		}

		statusOutput := captureStdout(t, func() {
			if err := run([]string{"status"}); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(statusOutput, "desktop: pass") {
			t.Fatalf("expected status to report desktop coverage, got:\n%s", statusOutput)
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

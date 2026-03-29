package proctor

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRecordBrowserRejectsTinyScreenshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))
	// 5-byte file — well under the 10KB minimum.
	tinyScreenshot := writeFixture(t, repo, "desktop.png", "image")

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": tinyScreenshot},
		PassAssertions: []string{"console_errors = 0"},
	})
	if err == nil {
		t.Fatal("expected error recording tiny screenshot, got nil")
	}
	if !strings.Contains(err.Error(), "too small") {
		t.Fatalf("expected error mentioning screenshot too small, got: %s", err.Error())
	}
}

func TestRecordCLIRejectsEmptyTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleCLIStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	screenshot := writeScreenshotFixture(t, repo, "terminal.png", "terminal-image")
	emptyTranscript := writeFixture(t, repo, "pane.txt", "")

	err = RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-1",
		Command:        "demo help",
		TranscriptPath: emptyTranscript,
		Screenshots:    map[string]string{"terminal": screenshot},
		PassAssertions: []string{"screenshot = true"},
	})
	if err == nil {
		t.Fatal("expected error recording empty transcript, got nil")
	}
	if !strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected error mentioning transcript too short, got: %s", err.Error())
	}
}

func TestRecordCLIRejectsNearEmptyTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleCLIStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	screenshot := writeScreenshotFixture(t, repo, "terminal.png", "terminal-image")
	// Just a few characters — not meaningful terminal output.
	tinyTranscript := writeFixture(t, repo, "pane.txt", "$ ")

	err = RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-1",
		Command:        "demo help",
		TranscriptPath: tinyTranscript,
		Screenshots:    map[string]string{"terminal": screenshot},
		PassAssertions: []string{"screenshot = true"},
	})
	if err == nil {
		t.Fatal("expected error recording near-empty transcript, got nil")
	}
	if !strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected error mentioning transcript too short, got: %s", err.Error())
	}
}

func TestCompleteRunRejectsExpiredRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	// Backdate the run's CreatedAt to 3 hours ago.
	run.CreatedAt = time.Now().UTC().Add(-3 * time.Hour)
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}

	_, err = CompleteRun(store, run)
	if err == nil {
		t.Fatal("expected error completing an expired run, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected error mentioning run expired, got: %s", err.Error())
	}
}

func TestCompleteRunAcceptsFreshRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	// Use the no-curl web options for simplicity.
	opts := sampleStartOptions()
	opts.CurlMode = "skip"
	opts.CurlSkipReason = "test only"
	opts.CurlEndpoints = nil
	run, err := CreateRun(store, repo, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Record enough evidence to satisfy the contract.
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))
	scenarioCounter := 0
	for _, scenario := range run.Scenarios {
		scenarioCounter++
		desktopShot := writeScreenshotFixture(t, repo, fmt.Sprintf("desktop-%d.png", scenarioCounter), fmt.Sprintf("desktop-image-%s", scenario.ID))
		mobileShot := writeScreenshotFixture(t, repo, fmt.Sprintf("mobile-%d.png", scenarioCounter), fmt.Sprintf("mobile-image-%s", scenario.ID))
		if err := RecordBrowser(store, run, BrowserRecordOptions{
			ScenarioID:     scenario.ID,
			SessionID:      "browser-1",
			ReportPath:     report,
			Screenshots:    map[string]string{"desktop": desktopShot, "mobile": mobileShot},
			PassAssertions: []string{"console_errors = 0"},
		}); err != nil {
			t.Fatalf("recording browser evidence for %s: %v", scenario.ID, err)
		}
	}

	// Run was just created — should not be expired.
	eval, err := CompleteRun(store, run)
	if err != nil {
		t.Fatalf("expected fresh run to complete, got: %v", err)
	}
	if !eval.Complete {
		t.Fatal("expected evaluation to be complete")
	}
}

func TestRecordBrowserRejectsCrossPlatformEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleCLIStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://localhost:3000", 0, 0, 0, 0))
	screenshot := writeFixture(t, repo, "desktop.png", "image")

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": screenshot},
		PassAssertions: []string{"console_errors = 0"},
	})
	if err == nil {
		t.Fatal("expected error recording browser evidence on a cli platform run, got nil")
	}
	if !strings.Contains(err.Error(), "not valid") || !strings.Contains(err.Error(), "cli") {
		t.Fatalf("expected error mentioning surface not valid for cli platform, got: %s", err.Error())
	}
}

func TestRecordIOSRejectsCrossPlatformEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "ios-report.json", `{
  "app": {"bundleId": "com.test.app", "screen": "main", "state": "running"},
  "issues": {"launchErrors": 0, "crashes": 0, "fatalLogs": 0}
}`)
	screenshot := writeFixture(t, repo, "ios.png", "image")

	err = RecordIOS(store, run, IOSRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "ios-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"main": screenshot},
		PassAssertions: []string{"bundle_id = com.test.app"},
	})
	if err == nil {
		t.Fatal("expected error recording ios evidence on a web platform run, got nil")
	}
	if !strings.Contains(err.Error(), "not valid") || !strings.Contains(err.Error(), "web") {
		t.Fatalf("expected error mentioning surface not valid for web platform, got: %s", err.Error())
	}
}

func TestRecordBrowserFailsAtRecordTimeWhenAssertionsFail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	// Report has 5 console errors — the assertion "console_errors = 0" should fail.
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 5, 0, 0, 0))
	screenshot := writeScreenshotFixture(t, repo, "desktop.png", "image")

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": screenshot},
		PassAssertions: []string{"console_errors = 0"},
	})
	if err == nil {
		t.Fatal("expected error when assertions fail at record time, got nil")
	}
	if !strings.Contains(err.Error(), "assertion failed") {
		t.Fatalf("expected assertion failure error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "console_errors") {
		t.Fatalf("expected error to mention which assertion failed, got: %s", err.Error())
	}
}

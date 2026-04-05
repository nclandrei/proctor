package proctor

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
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

func TestCompleteRunRejectsDuplicateScreenshots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	opts := sampleStartOptions()
	opts.CurlMode = "skip"
	opts.CurlSkipReason = "test only"
	opts.CurlEndpoints = nil
	run, err := CreateRun(store, repo, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Write one screenshot file and reuse its SHA256 for two scenarios.
	screenshotContent := writeScreenshotFixture(t, repo, "shared.png", "identical-image-content")

	// Compute SHA256 of the screenshot file.
	data, err := os.ReadFile(screenshotContent)
	if err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Copy the screenshot into the run directory for both scenarios.
	runDir := store.RunDir(run)
	for i, scenario := range run.Scenarios {
		artPath := fmt.Sprintf("evidence/screenshot-%d.png", i)
		dst := filepath.Join(runDir, artPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}

		evidence := Evidence{
			ID:         fmt.Sprintf("ev-%d", i),
			RunID:      run.ID,
			ScenarioID: scenario.ID,
			Surface:    SurfaceBrowser,
			Tier:       TierRegisteredRun,
			CreatedAt:  time.Now().UTC(),
			Title:      fmt.Sprintf("browser check for %s", scenario.ID),
			Provenance: Provenance{Mode: "registered-session", Tool: "playwright"},
			Artifacts: []Artifact{
				{Kind: ArtifactImage, Label: "desktop", Path: artPath, SHA256: hash},
			},
		}
		if err := store.AppendEvidence(run, evidence); err != nil {
			t.Fatal(err)
		}
	}

	// Evaluate should detect the duplicate.
	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if eval.Complete {
		t.Fatal("expected eval.Complete == false when screenshots are duplicated across scenarios")
	}
	if !containsSubstring(eval.GlobalMissing, "duplicate screenshot") {
		t.Fatalf("expected GlobalMissing to mention duplicate screenshot, got %#v", eval.GlobalMissing)
	}

	// CompleteRun should mark the run as blocked.
	eval, err = CompleteRun(store, run)
	if err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}
	reloaded, err := store.LoadRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusBlocked {
		t.Fatalf("expected run status %q, got %q", StatusBlocked, reloaded.Status)
	}
}

// captureForTest appends a CaptureRecord for the given surface/scenario/session
// with the specified SHA and returns the record. Tests use this helper to
// target arbitrary SHAs without writing real artifact files.
func captureForTest(t *testing.T, store *Store, run Run, surface, scenarioID, sessionID, sha string) CaptureRecord {
	t.Helper()
	id, err := GenerateCaptureID()
	if err != nil {
		t.Fatal(err)
	}
	rec := CaptureRecord{
		ID:             id,
		RunID:          run.ID,
		ScenarioID:     scenarioID,
		SessionID:      sessionID,
		Surface:        surface,
		Label:          "main",
		ArtifactPath:   "/dev/null",
		ArtifactSHA256: sha,
		ArtifactBytes:  1,
		Verification:   CaptureVerifyNone,
		CapturedAt:     time.Now().UTC(),
	}
	if err := store.CaptureLedger(run).Append(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func shaOfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

func TestRecordBrowserWithValidCaptureIDSucceeds(t *testing.T) {
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

	desktop := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobile := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))

	rec := captureForTest(t, store, run, SurfaceBrowser, "happy-path", "browser-1", shaOfFile(t, desktop))

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop, "mobile": mobile},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordBrowserWithoutCaptureIDStillSucceeds(t *testing.T) {
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

	desktop := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordBrowserWithUnknownCaptureIDFails(t *testing.T) {
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

	desktop := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      "cap_UNKNOWN",
	})
	if err == nil {
		t.Fatal("expected error for unknown capture id")
	}
	if !strings.Contains(err.Error(), "not found in ledger") {
		t.Fatalf("expected ledger miss error, got %v", err)
	}
}

func TestRecordBrowserWithCaptureIDForWrongScenarioFails(t *testing.T) {
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

	desktop := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))
	rec := captureForTest(t, store, run, SurfaceBrowser, "failure-path", "browser-1", shaOfFile(t, desktop))

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected error for wrong scenario")
	}
	if !strings.Contains(err.Error(), "belongs to scenario") {
		t.Fatalf("expected scenario mismatch error, got %v", err)
	}
}

func TestRecordBrowserWithCaptureIDForWrongSessionFails(t *testing.T) {
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

	desktop := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))
	rec := captureForTest(t, store, run, SurfaceBrowser, "happy-path", "other-session", shaOfFile(t, desktop))

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected error for wrong session")
	}
	if !strings.Contains(err.Error(), "belongs to session") {
		t.Fatalf("expected session mismatch error, got %v", err)
	}
}

func TestRecordBrowserWithCaptureIDForWrongSurfaceFails(t *testing.T) {
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

	desktop := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))
	rec := captureForTest(t, store, run, SurfaceDesktop, "happy-path", "browser-1", shaOfFile(t, desktop))

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected error for wrong surface")
	}
	if !strings.Contains(err.Error(), "different surface") && !strings.Contains(err.Error(), "was for surface") {
		t.Fatalf("expected surface mismatch error, got %v", err)
	}
}

func TestRecordBrowserWithTamperedArtifactFails(t *testing.T) {
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

	desktop := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 0))
	// Capture was bound to a totally different SHA — the submitted PNG is tampered relative to the ledger.
	rec := captureForTest(t, store, run, SurfaceBrowser, "happy-path", "browser-1", "0000000000000000000000000000000000000000000000000000000000000000")
	_ = desktop

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected error for tampered artifact")
	}
	if !strings.Contains(err.Error(), "no submitted artifact matches capture") {
		t.Fatalf("expected tamper error, got %v", err)
	}
}

func TestRecordIOSWithValidCaptureIDSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleIOSStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	screenshot := writeScreenshotFixture(t, repo, "library.png", "ios-image")
	report := writeFixture(t, repo, "ios-report.json", sampleIOSReport("com.example.pagena", "Library", "foreground", "iPhone 16 Pro", "iOS 18.2", 0, 0, 0))
	rec := captureForTest(t, store, run, SurfaceIOS, "happy-path", "ios-1", shaOfFile(t, screenshot))

	if err := RecordIOS(store, run, IOSRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "ios-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"library": screenshot},
		PassAssertions: []string{"bundle_id = com.example.pagena"},
		CaptureID:      rec.ID,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordIOSWithUnknownCaptureIDFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleIOSStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	screenshot := writeScreenshotFixture(t, repo, "library.png", "ios-image")
	report := writeFixture(t, repo, "ios-report.json", sampleIOSReport("com.example.pagena", "Library", "foreground", "iPhone 16 Pro", "iOS 18.2", 0, 0, 0))

	err = RecordIOS(store, run, IOSRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "ios-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"library": screenshot},
		PassAssertions: []string{"bundle_id = com.example.pagena"},
		CaptureID:      "cap_ZZZZZZ",
	})
	if err == nil {
		t.Fatal("expected error for unknown capture id")
	}
	if !strings.Contains(err.Error(), "not found in ledger") {
		t.Fatalf("expected ledger miss error, got %v", err)
	}
}

func TestRecordDesktopWithValidCaptureIDSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleDesktopStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	screenshot := writeScreenshotFixture(t, repo, "window.png", "window-image")
	report := writeFixture(t, repo, "desktop-report.json", sampleDesktopReport("Firefox", "org.mozilla.firefox", "running", "Bookmark Manager", 0, 0))
	rec := captureForTest(t, store, run, SurfaceDesktop, "happy-path", "desktop-1", shaOfFile(t, screenshot))

	if err := RecordDesktop(store, run, DesktopRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "desktop-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"window": screenshot},
		PassAssertions: []string{"app_name contains Firefox"},
		CaptureID:      rec.ID,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordDesktopWithUnknownCaptureIDFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleDesktopStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	screenshot := writeScreenshotFixture(t, repo, "window.png", "window-image")
	report := writeFixture(t, repo, "desktop-report.json", sampleDesktopReport("Firefox", "org.mozilla.firefox", "running", "Bookmark Manager", 0, 0))

	err = RecordDesktop(store, run, DesktopRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "desktop-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"window": screenshot},
		PassAssertions: []string{"app_name contains Firefox"},
		CaptureID:      "cap_XXXXXX",
	})
	if err == nil {
		t.Fatal("expected error for unknown capture id")
	}
	if !strings.Contains(err.Error(), "not found in ledger") {
		t.Fatalf("expected ledger miss error, got %v", err)
	}
}

func TestRecordCLIWithValidCaptureIDSucceeds(t *testing.T) {
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

	terminal := writeScreenshotFixture(t, repo, "terminal.png", "terminal-image")
	transcript := writeFixture(t, repo, "pane.txt", "Usage:\n  demo help\nOnboarding prompt output\n")
	rec := captureForTest(t, store, run, SurfaceCLI, "happy-path", "cli-1", shaOfFile(t, terminal))

	if err := RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-1",
		Command:        "demo help",
		TranscriptPath: transcript,
		Screenshots:    map[string]string{"terminal": terminal},
		PassAssertions: []string{"screenshot = true"},
		CaptureID:      rec.ID,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordCLIWithUnknownCaptureIDFails(t *testing.T) {
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

	terminal := writeScreenshotFixture(t, repo, "terminal.png", "terminal-image")
	transcript := writeFixture(t, repo, "pane.txt", "Usage:\n  demo help\nOnboarding prompt output\n")

	err = RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-1",
		Command:        "demo help",
		TranscriptPath: transcript,
		Screenshots:    map[string]string{"terminal": terminal},
		PassAssertions: []string{"screenshot = true"},
		CaptureID:      "cap_MISSING",
	})
	if err == nil {
		t.Fatal("expected error for unknown capture id")
	}
	if !strings.Contains(err.Error(), "not found in ledger") {
		t.Fatalf("expected ledger miss error, got %v", err)
	}
}

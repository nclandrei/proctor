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

func TestRecordBrowserRejectsMissingPreNote(t *testing.T) {
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

	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/dashboard", 0, 0, 0, 0))
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desk-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "no-prenote-session",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktopShot, "mobile": mobileShot},
		PassAssertions: []string{"final_url contains /dashboard"},
	})
	if err == nil {
		t.Fatal("expected RecordBrowser to refuse evidence without a pre-note")
	}
	if !strings.Contains(err.Error(), "file a pre-test note first") {
		t.Fatalf("expected pre-note gate error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "proctor note --scenario happy-path --session no-prenote-session") {
		t.Fatalf("expected gate error to include the concrete proctor note hint, got: %v", err)
	}
}

func TestRecordIOSRejectsMissingPreNote(t *testing.T) {
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

	report := writeFixture(t, repo, "ios-report.json", sampleIOSReport("com.example.pagena", "Library", "foreground", "iPhone 16 Pro", "iOS 18.2", 0, 0, 0))
	screenshot := writeScreenshotFixture(t, repo, "library.png", "library-image")

	err = RecordIOS(store, run, IOSRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "ios-no-prenote",
		ReportPath:     report,
		Screenshots:    map[string]string{"library": screenshot},
		PassAssertions: []string{"bundle_id = com.example.pagena"},
	})
	if err == nil {
		t.Fatal("expected RecordIOS to refuse evidence without a pre-note")
	}
	if !strings.Contains(err.Error(), "file a pre-test note first") {
		t.Fatalf("expected pre-note gate error, got: %v", err)
	}
}

func TestRecordDesktopRejectsMissingPreNote(t *testing.T) {
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

	report := writeFixture(t, repo, "desktop-report.json", sampleDesktopReport("Firefox", "org.mozilla.firefox", "running", "Bookmark Manager", 0, 0))
	screenshot := writeScreenshotFixture(t, repo, "window.png", "window-image")

	err = RecordDesktop(store, run, DesktopRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "desktop-no-prenote",
		ReportPath:     report,
		Screenshots:    map[string]string{"window": screenshot},
		PassAssertions: []string{"app_name contains Firefox"},
	})
	if err == nil {
		t.Fatal("expected RecordDesktop to refuse evidence without a pre-note")
	}
	if !strings.Contains(err.Error(), "file a pre-test note first") {
		t.Fatalf("expected pre-note gate error, got: %v", err)
	}
}

func TestRecordCLIRejectsMissingPreNote(t *testing.T) {
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
	transcript := writeFixture(t, repo, "pane.txt", "Usage:\n  demo help\nOnboarding prompt output\n")

	err = RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-no-prenote",
		Command:        "demo help",
		TranscriptPath: transcript,
		Screenshots:    map[string]string{"terminal": screenshot},
		PassAssertions: []string{"screenshot = true"},
	})
	if err == nil {
		t.Fatal("expected RecordCLI to refuse evidence without a pre-note")
	}
	if !strings.Contains(err.Error(), "file a pre-test note first") {
		t.Fatalf("expected pre-note gate error, got: %v", err)
	}
}

func TestRecordCurlRejectsMissingPreNote(t *testing.T) {
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

	err = RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "happy-path",
		Command: []string{
			"sh", "-c",
			`printf 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{"ok":true}'`,
		},
		PassAssertions: []string{"status = 200"},
	})
	if err == nil {
		t.Fatal("expected RecordCurl to refuse evidence without a pre-note")
	}
	if !strings.Contains(err.Error(), "file a pre-test note first") {
		t.Fatalf("expected pre-note gate error, got: %v", err)
	}
}

func TestDoneBlocksWhenPreNoteMissingForRecordedEvidence(t *testing.T) {
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

	// Synthesize a browser evidence record directly, bypassing the record
	// gate, to simulate a legacy ledger where evidence exists without a
	// pre-note.
	shot := writeScreenshotFixture(t, repo, "desktop.png", "image")
	artifact, err := store.CopyArtifact(run, SurfaceBrowser, "happy-path", "desktop", shot)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Kind = ArtifactImage
	evidence := Evidence{
		ID:         newID("ev"),
		RunID:      run.ID,
		ScenarioID: "happy-path",
		Surface:    SurfaceBrowser,
		Tier:       TierRegisteredRun,
		CreatedAt:  time.Now().UTC(),
		Title:      "legacy browser verification",
		Provenance: Provenance{Mode: "registered-session", Tool: "legacy", SessionID: "legacy-session", CWD: run.RepoRoot, RecordedBy: "proctor"},
		Assertions: []Assertion{{Description: "console_errors = 0", Result: AssertionPass}},
		Artifacts:  []Artifact{artifact},
		Status:     EvidenceStatusComplete,
	}
	if err := store.AppendEvidence(run, evidence); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.Complete {
		t.Fatalf("expected evaluation to remain incomplete with missing pre-note, got %#v", eval)
	}
	var happyPath ScenarioEvaluation
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.ID == "happy-path" {
			happyPath = item
			break
		}
	}
	if !containsSubstring(happyPath.BrowserIssues, "has evidence but no pre-test note recorded") {
		t.Fatalf("expected pre-note gap message, got %#v", happyPath.BrowserIssues)
	}
}

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

	filePreNote(t, store, run, "happy-path", "browser-1")
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

	filePreNote(t, store, run, "happy-path", "cli-1")
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

	filePreNote(t, store, run, "happy-path", "cli-1")
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

	filePreNotesForAll(t, store, run, "browser-1", testPreNoteText)

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

	verifyAllScenarios(t, store, run, testObservationNotes)

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

	filePreNote(t, store, run, "happy-path", "browser-1")
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

func shaOfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// latestEvidence returns the most recently appended evidence record for
// tests that inspect the auto-populated CaptureIDs and ledger entries.
func latestEvidence(t *testing.T, store *Store, run Run) Evidence {
	t.Helper()
	items, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one evidence record")
	}
	return items[len(items)-1]
}

func TestRecordBrowserAutoWritesLedgerEntryPerImage(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop, "mobile": mobile},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev := latestEvidence(t, store, run)
	if len(ev.CaptureIDs) != 2 {
		t.Fatalf("expected 2 capture ids on evidence, got %d: %v", len(ev.CaptureIDs), ev.CaptureIDs)
	}

	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", len(records))
	}
	labelShas := map[string]string{}
	for _, r := range records {
		if r.ScenarioID != "happy-path" {
			t.Errorf("wrong scenario: %s", r.ScenarioID)
		}
		if r.SessionID != "browser-1" {
			t.Errorf("wrong session: %s", r.SessionID)
		}
		if r.Surface != SurfaceBrowser {
			t.Errorf("wrong surface: %s", r.Surface)
		}
		if r.ArtifactSHA256 == "" {
			t.Errorf("missing sha")
		}
		labelShas[r.Label] = r.ArtifactSHA256
	}
	if labelShas["desktop"] != shaOfFile(t, desktop) {
		t.Errorf("desktop ledger sha mismatch: %s vs %s", labelShas["desktop"], shaOfFile(t, desktop))
	}
	if labelShas["mobile"] != shaOfFile(t, mobile) {
		t.Errorf("mobile ledger sha mismatch: %s vs %s", labelShas["mobile"], shaOfFile(t, mobile))
	}

	// Every capture id on the evidence should resolve via the ledger.
	for _, id := range ev.CaptureIDs {
		rec, ok, err := store.CaptureLedger(run).FindByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("capture id %s missing from ledger", id)
		}
		if rec.Surface != SurfaceBrowser {
			t.Errorf("ledger record %s wrong surface: %s", id, rec.Surface)
		}
	}
}

func TestRecordIOSAutoWritesLedgerEntryPerImage(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "ios-1")
	if err := RecordIOS(store, run, IOSRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "ios-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"library": screenshot},
		PassAssertions: []string{"bundle_id = com.example.pagena"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev := latestEvidence(t, store, run)
	if len(ev.CaptureIDs) != 1 {
		t.Fatalf("expected 1 capture id on evidence, got %d", len(ev.CaptureIDs))
	}
	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(records))
	}
	r := records[0]
	if r.Surface != SurfaceIOS || r.ScenarioID != "happy-path" || r.SessionID != "ios-1" {
		t.Errorf("bad ledger record: %+v", r)
	}
	if r.Label != "library" {
		t.Errorf("expected label library, got %s", r.Label)
	}
	if r.ArtifactSHA256 != shaOfFile(t, screenshot) {
		t.Errorf("sha mismatch: %s vs %s", r.ArtifactSHA256, shaOfFile(t, screenshot))
	}
}

func TestRecordDesktopAutoWritesLedgerEntryPerImage(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "desktop-1")
	if err := RecordDesktop(store, run, DesktopRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "desktop-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"window": screenshot},
		PassAssertions: []string{"app_name contains Firefox"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev := latestEvidence(t, store, run)
	if len(ev.CaptureIDs) != 1 {
		t.Fatalf("expected 1 capture id on evidence, got %d", len(ev.CaptureIDs))
	}
	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(records))
	}
	r := records[0]
	if r.Surface != SurfaceDesktop || r.ScenarioID != "happy-path" || r.SessionID != "desktop-1" {
		t.Errorf("bad ledger record: %+v", r)
	}
	if r.Label != "window" {
		t.Errorf("expected label window, got %s", r.Label)
	}
	if r.ArtifactSHA256 != shaOfFile(t, screenshot) {
		t.Errorf("sha mismatch: %s vs %s", r.ArtifactSHA256, shaOfFile(t, screenshot))
	}
}

func TestRecordCLIAutoWritesLedgerEntryPerImage(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "cli-1")
	if err := RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-1",
		Command:        "demo help",
		TranscriptPath: transcript,
		Screenshots:    map[string]string{"terminal": terminal},
		PassAssertions: []string{"screenshot = true"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev := latestEvidence(t, store, run)
	if len(ev.CaptureIDs) != 1 {
		t.Fatalf("expected 1 capture id on evidence, got %d", len(ev.CaptureIDs))
	}
	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(records))
	}
	r := records[0]
	if r.Surface != SurfaceCLI || r.ScenarioID != "happy-path" || r.SessionID != "cli-1" {
		t.Errorf("bad ledger record: %+v", r)
	}
	if r.Label != "terminal" {
		t.Errorf("expected label terminal, got %s", r.Label)
	}
	if r.ArtifactSHA256 != shaOfFile(t, terminal) {
		t.Errorf("sha mismatch: %s vs %s", r.ArtifactSHA256, shaOfFile(t, terminal))
	}
}

func TestRecordBrowserTamperDetectedViaLedger(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev := latestEvidence(t, store, run)
	if len(ev.CaptureIDs) != 1 {
		t.Fatalf("expected 1 capture id, got %d", len(ev.CaptureIDs))
	}
	captureID := ev.CaptureIDs[0]

	// Look up the ledger entry directly and compare its SHA against the
	// copied artifact SHA. They must agree — tampering would surface as a
	// mismatch between these two values.
	rec, ok, err := store.CaptureLedger(run).FindByID(captureID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("capture %s missing from ledger", captureID)
	}

	// Match the ledger record's SHA against the evidence artifact SHA.
	var imgSHA string
	for _, art := range ev.Artifacts {
		if art.Kind == ArtifactImage {
			imgSHA = art.SHA256
			break
		}
	}
	if imgSHA == "" {
		t.Fatal("evidence has no image artifact")
	}
	if rec.ArtifactSHA256 != imgSHA {
		t.Fatalf("tamper: ledger sha %s != evidence sha %s", rec.ArtifactSHA256, imgSHA)
	}

	// Now simulate tampering: mutate the ledger entry's SHA and
	// verifyCaptureBinding should reject the original artifacts.
	tamperedRec := rec
	tamperedRec.ArtifactSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	err = verifyCaptureBinding(store, run, "happy-path", "browser-1", SurfaceBrowser, rec.ID, []Artifact{{SHA256: tamperedRec.ArtifactSHA256}})
	// verifyCaptureBinding takes the ledger's own record, so testing the
	// public helper with a fabricated SHA lets us exercise its tamper
	// branch directly.
	if err == nil {
		// The artifact we submitted matches the fabricated SHA; swap to a
		// definitely-different one and retry.
		err = verifyCaptureBinding(store, run, "happy-path", "browser-1", SurfaceBrowser, rec.ID, []Artifact{{SHA256: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}})
	}
	if err == nil {
		t.Fatal("expected verifyCaptureBinding to reject mismatched artifact")
	}
	if !strings.Contains(err.Error(), "no submitted artifact matches capture") {
		t.Fatalf("expected tamper error, got %v", err)
	}
}

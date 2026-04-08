package proctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupLogFixture(t *testing.T) (*Store, Run, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-log-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}
	return store, run, repo
}

func TestLogStepHappyPath(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	shot := writeScreenshotFixture(t, repo, "step1.png", "step1-login-page")

	entry, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "log-session-1",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot,
		Action:         "Navigated to the login page and saw the login form",
	})
	if err != nil {
		t.Fatalf("LogStep failed: %v", err)
	}
	if entry.Step != 1 {
		t.Fatalf("expected step 1, got %d", entry.Step)
	}
	if entry.ScenarioID != "happy-path" {
		t.Fatalf("expected scenario happy-path, got %s", entry.ScenarioID)
	}
	if entry.SessionID != "log-session-1" {
		t.Fatalf("expected session log-session-1, got %s", entry.SessionID)
	}
	if entry.Surface != SurfaceBrowser {
		t.Fatalf("expected surface browser, got %s", entry.Surface)
	}
	if entry.SHA256 == "" {
		t.Fatal("expected SHA256 to be set")
	}
	if !strings.HasPrefix(entry.ID, "log_") {
		t.Fatalf("expected ID to start with log_, got %s", entry.ID)
	}

	// Verify artifact was copied.
	artPath := filepath.Join(store.RunDir(run), entry.ScreenshotPath)
	if _, err := os.Stat(artPath); err != nil {
		t.Fatalf("screenshot artifact not found at %s: %v", artPath, err)
	}
}

func TestLogStepAutoIncrementsSteps(t *testing.T) {
	store, run, repo := setupLogFixture(t)

	for i := 1; i <= 3; i++ {
		shot := writeScreenshotFixture(t, repo, "step.png", "step-image-unique-"+itoa(i))
		entry, err := LogStep(store, run, LogStepOptions{
			ScenarioID:     "happy-path",
			SessionID:      "log-session-1",
			Surface:        SurfaceBrowser,
			ScreenshotPath: shot,
			Action:         "Step action description for step " + itoa(i),
		})
		if err != nil {
			t.Fatalf("LogStep %d failed: %v", i, err)
		}
		if entry.Step != i {
			t.Fatalf("expected step %d, got %d", i, entry.Step)
		}
	}
}

func TestLogStepSeparateSessionsGetSeparateStepCounters(t *testing.T) {
	store, run, repo := setupLogFixture(t)

	shot1 := writeScreenshotFixture(t, repo, "s1.png", "session1-image")
	entry1, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "session-A",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot1,
		Action:         "First action in session A for the test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry1.Step != 1 {
		t.Fatalf("session-A step: expected 1, got %d", entry1.Step)
	}

	shot2 := writeScreenshotFixture(t, repo, "s2.png", "session2-image")
	entry2, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "session-B",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot2,
		Action:         "First action in session B for the test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry2.Step != 1 {
		t.Fatalf("session-B step: expected 1, got %d", entry2.Step)
	}
}

func TestLogStepRequiresAllFields(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	shot := writeScreenshotFixture(t, repo, "req.png", "required-test")

	tests := []struct {
		name string
		opts LogStepOptions
		want string
	}{
		{
			name: "missing scenario",
			opts: LogStepOptions{SessionID: "s", Surface: "browser", ScreenshotPath: shot, Action: "action text that is long enough"},
			want: "--scenario is required",
		},
		{
			name: "missing session",
			opts: LogStepOptions{ScenarioID: "happy-path", Surface: "browser", ScreenshotPath: shot, Action: "action text that is long enough"},
			want: "--session is required",
		},
		{
			name: "missing surface",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", ScreenshotPath: shot, Action: "action text that is long enough"},
			want: "--surface is required",
		},
		{
			name: "missing screenshot",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", Surface: "browser", Action: "action text that is long enough"},
			want: "--screenshot is required",
		},
		{
			name: "missing action",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", Surface: "browser", ScreenshotPath: shot},
			want: "--action is required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LogStep(store, run, tc.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestLogStepRequiresMinActionLength(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	shot := writeScreenshotFixture(t, repo, "min.png", "min-test")

	_, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "s",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot,
		Action:         "too short",
	})
	if err == nil {
		t.Fatal("expected short action to be rejected")
	}
	if !strings.Contains(err.Error(), "must be specific") {
		t.Fatalf("expected specificity error, got: %v", err)
	}
}

func TestLogStepRejectsUnknownScenario(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	shot := writeScreenshotFixture(t, repo, "unk.png", "unknown-test")

	_, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "nonexistent-scenario",
		SessionID:      "s",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot,
		Action:         "Navigated to the login page and saw the form",
	})
	if err == nil {
		t.Fatal("expected unknown scenario to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("expected unknown scenario error, got: %v", err)
	}
}

func TestScreenshotLogLedgerLoad(t *testing.T) {
	store, run, repo := setupLogFixture(t)

	// Empty ledger returns nil.
	ledger := store.ScreenshotLogLedger(run)
	entries, err := ledger.Load()
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Fatalf("expected nil for empty ledger, got %d entries", len(entries))
	}

	// Add two entries.
	for i := 0; i < 2; i++ {
		shot := writeScreenshotFixture(t, repo, "load-"+itoa(i)+".png", "load-image-"+itoa(i))
		if _, err := LogStep(store, run, LogStepOptions{
			ScenarioID:     "happy-path",
			SessionID:      "load-session",
			Surface:        SurfaceBrowser,
			ScreenshotPath: shot,
			Action:         "Loading test step action " + itoa(i) + " with enough chars",
		}); err != nil {
			t.Fatal(err)
		}
	}

	entries, err = ledger.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Step != 1 || entries[1].Step != 2 {
		t.Fatalf("expected steps 1 and 2, got %d and %d", entries[0].Step, entries[1].Step)
	}
}

func TestScreenshotLogLedgerLoadForScenario(t *testing.T) {
	store, run, repo := setupLogFixture(t)

	// Add entries for two scenarios.
	shot1 := writeScreenshotFixture(t, repo, "hp.png", "happy-path-image")
	if _, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "s1",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot1,
		Action:         "Happy path step with enough characters here",
	}); err != nil {
		t.Fatal(err)
	}

	shot2 := writeScreenshotFixture(t, repo, "fp.png", "failure-path-image")
	if _, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "failure-path",
		SessionID:      "s2",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot2,
		Action:         "Failure path step with enough characters here",
	}); err != nil {
		t.Fatal(err)
	}

	ledger := store.ScreenshotLogLedger(run)
	entries, err := ledger.LoadForScenario("happy-path")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for happy-path, got %d", len(entries))
	}
	if entries[0].ScenarioID != "happy-path" {
		t.Fatalf("expected happy-path, got %s", entries[0].ScenarioID)
	}
}

func TestAnalysisLedgerAppendAndLoad(t *testing.T) {
	store, run, _ := setupLogFixture(t)

	ledger := store.AnalysisLedger(run)

	// Empty ledger returns nil.
	records, err := ledger.Load()
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Fatalf("expected nil for empty ledger, got %d", len(records))
	}

	// Append an analysis.
	analysis := VisionAnalysis{
		ID:            "va_test_1",
		RunID:         run.ID,
		ScenarioID:    "happy-path",
		Description:   "Login page with email and password fields",
		Comparison:    "Matches the expected login form layout",
		Findings:      []string{"Email field present", "Password field present"},
		Concerns:      []string{},
		MatchesIntent: true,
		Model:         "claude-sonnet-4-20250514",
	}
	if err := ledger.Append(analysis); err != nil {
		t.Fatal(err)
	}

	records, err = ledger.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].ID != "va_test_1" {
		t.Fatalf("expected ID va_test_1, got %s", records[0].ID)
	}
	if !records[0].MatchesIntent {
		t.Fatal("expected MatchesIntent to be true")
	}
	if len(records[0].Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(records[0].Findings))
	}
}

func TestAnalysisLedgerLoadForScenario(t *testing.T) {
	store, run, _ := setupLogFixture(t)

	ledger := store.AnalysisLedger(run)
	if err := ledger.Append(VisionAnalysis{
		ID: "va_1", RunID: run.ID, ScenarioID: "happy-path",
		Description: "happy path view", Comparison: "matches", Findings: []string{"ok"},
		Concerns: []string{}, MatchesIntent: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Append(VisionAnalysis{
		ID: "va_2", RunID: run.ID, ScenarioID: "failure-path",
		Description: "failure view", Comparison: "shows error", Findings: []string{"error visible"},
		Concerns: []string{}, MatchesIntent: true,
	}); err != nil {
		t.Fatal(err)
	}

	filtered, err := ledger.LoadForScenario("failure-path")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 record for failure-path, got %d", len(filtered))
	}
	if filtered[0].ID != "va_2" {
		t.Fatalf("expected va_2, got %s", filtered[0].ID)
	}
}

func TestScreenshotLogPath(t *testing.T) {
	store, run, _ := setupLogFixture(t)

	path := store.ScreenshotLogPath(run)
	if !strings.HasSuffix(path, "screenshot-log.jsonl") {
		t.Fatalf("expected path to end with screenshot-log.jsonl, got %s", path)
	}
}

func TestAnalysisPath(t *testing.T) {
	store, run, _ := setupLogFixture(t)

	path := store.AnalysisPath(run)
	if !strings.HasSuffix(path, "analysis.jsonl") {
		t.Fatalf("expected path to end with analysis.jsonl, got %s", path)
	}
}

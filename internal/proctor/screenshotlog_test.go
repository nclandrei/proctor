package proctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testAction      = "Navigated to the login page at http://127.0.0.1:3000/login"
	testObservation = "Login form visible with email field, password field, and blue Sign In button"
	testComparison  = "Login page matches expected layout; ready to enter credentials for the happy-path scenario"
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

func logOpts(repo, shotName, shotContent string, t *testing.T) LogStepOptions {
	t.Helper()
	shot := writeScreenshotFixture(t, repo, shotName, shotContent)
	return LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "log-session-1",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot,
		Action:         testAction,
		Observation:    testObservation,
		Comparison:     testComparison,
	}
}

// --- LogStep tests ---

func TestLogStepHappyPath(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	opts := logOpts(repo, "step1.png", "step1-login-page", t)

	entry, err := LogStep(store, run, opts)
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
	if entry.Action != testAction {
		t.Fatalf("expected action %q, got %q", testAction, entry.Action)
	}
	if entry.Observation != testObservation {
		t.Fatalf("expected observation %q, got %q", testObservation, entry.Observation)
	}
	if entry.Comparison != testComparison {
		t.Fatalf("expected comparison %q, got %q", testComparison, entry.Comparison)
	}
	if entry.SHA256 == "" {
		t.Fatal("expected SHA256 to be set")
	}
	if !strings.HasPrefix(entry.ID, "log_") {
		t.Fatalf("expected ID to start with log_, got %s", entry.ID)
	}
	if entry.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
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
		opts := logOpts(repo, "step.png", "step-image-unique-"+itoa(i), t)
		entry, err := LogStep(store, run, opts)
		if err != nil {
			t.Fatalf("LogStep %d failed: %v", i, err)
		}
		if entry.Step != i {
			t.Fatalf("expected step %d, got %d", i, entry.Step)
		}
	}
}

func TestLogStepSeparateSessionsGetSeparateCounters(t *testing.T) {
	store, run, repo := setupLogFixture(t)

	opts1 := logOpts(repo, "s1.png", "session1-image", t)
	opts1.SessionID = "session-A"
	entry1, err := LogStep(store, run, opts1)
	if err != nil {
		t.Fatal(err)
	}
	if entry1.Step != 1 {
		t.Fatalf("session-A step: expected 1, got %d", entry1.Step)
	}

	opts2 := logOpts(repo, "s2.png", "session2-image", t)
	opts2.SessionID = "session-B"
	entry2, err := LogStep(store, run, opts2)
	if err != nil {
		t.Fatal(err)
	}
	if entry2.Step != 1 {
		t.Fatalf("session-B step: expected 1, got %d", entry2.Step)
	}
}

func TestLogStepSeparateScenariosGetSeparateCounters(t *testing.T) {
	store, run, repo := setupLogFixture(t)

	opts1 := logOpts(repo, "hp.png", "happy-image", t)
	entry1, err := LogStep(store, run, opts1)
	if err != nil {
		t.Fatal(err)
	}
	if entry1.Step != 1 {
		t.Fatalf("happy-path step: expected 1, got %d", entry1.Step)
	}

	opts2 := logOpts(repo, "fp.png", "failure-image", t)
	opts2.ScenarioID = "failure-path"
	entry2, err := LogStep(store, run, opts2)
	if err != nil {
		t.Fatal(err)
	}
	if entry2.Step != 1 {
		t.Fatalf("failure-path step: expected 1, got %d", entry2.Step)
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
			opts: LogStepOptions{SessionID: "s", Surface: "browser", ScreenshotPath: shot, Action: testAction, Observation: testObservation, Comparison: testComparison},
			want: "--scenario is required",
		},
		{
			name: "missing session",
			opts: LogStepOptions{ScenarioID: "happy-path", Surface: "browser", ScreenshotPath: shot, Action: testAction, Observation: testObservation, Comparison: testComparison},
			want: "--session is required",
		},
		{
			name: "missing surface",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", ScreenshotPath: shot, Action: testAction, Observation: testObservation, Comparison: testComparison},
			want: "--surface is required",
		},
		{
			name: "missing screenshot",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", Surface: "browser", Action: testAction, Observation: testObservation, Comparison: testComparison},
			want: "--screenshot is required",
		},
		{
			name: "missing action",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", Surface: "browser", ScreenshotPath: shot, Observation: testObservation, Comparison: testComparison},
			want: "--action is required",
		},
		{
			name: "missing observation",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", Surface: "browser", ScreenshotPath: shot, Action: testAction, Comparison: testComparison},
			want: "--observation is required",
		},
		{
			name: "missing comparison",
			opts: LogStepOptions{ScenarioID: "happy-path", SessionID: "s", Surface: "browser", ScreenshotPath: shot, Action: testAction, Observation: testObservation},
			want: "--comparison is required",
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

func TestLogStepRequiresMinLengths(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	shot := writeScreenshotFixture(t, repo, "min.png", "min-test")

	tests := []struct {
		name   string
		action string
		obs    string
		comp   string
		want   string
	}{
		{"short action", "too short", testObservation, testComparison, "action must describe what you did"},
		{"short observation", testAction, "too short", testComparison, "observation must describe what you see"},
		{"short comparison", testAction, testObservation, "too short", "comparison must explain"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LogStep(store, run, LogStepOptions{
				ScenarioID: "happy-path", SessionID: "s", Surface: SurfaceBrowser,
				ScreenshotPath: shot, Action: tc.action, Observation: tc.obs, Comparison: tc.comp,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestLogStepRejectsUnknownScenario(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	opts := logOpts(repo, "unk.png", "unknown-test", t)
	opts.ScenarioID = "nonexistent-scenario"

	_, err := LogStep(store, run, opts)
	if err == nil {
		t.Fatal("expected unknown scenario to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("expected unknown scenario error, got: %v", err)
	}
}

func TestLogStepTrimsWhitespace(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	shot := writeScreenshotFixture(t, repo, "trim.png", "trim-test")
	entry, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "  happy-path  ",
		SessionID:      "  log-session-1  ",
		Surface:        "  browser  ",
		ScreenshotPath: shot,
		Action:         "  " + testAction + "  ",
		Observation:    "  " + testObservation + "  ",
		Comparison:     "  " + testComparison + "  ",
	})
	if err != nil {
		t.Fatalf("LogStep failed: %v", err)
	}
	if entry.ScenarioID != "happy-path" {
		t.Fatalf("expected trimmed scenario, got %q", entry.ScenarioID)
	}
	if entry.Action != testAction {
		t.Fatalf("expected trimmed action, got %q", entry.Action)
	}
	if entry.Observation != testObservation {
		t.Fatalf("expected trimmed observation, got %q", entry.Observation)
	}
}

// --- Ledger tests ---

func TestScreenshotLogLedgerEmptyLoad(t *testing.T) {
	store, run, _ := setupLogFixture(t)
	ledger := store.ScreenshotLogLedger(run)

	entries, err := ledger.Load()
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Fatalf("expected nil for empty ledger, got %d entries", len(entries))
	}
}

func TestScreenshotLogLedgerRoundTrip(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	ledger := store.ScreenshotLogLedger(run)

	for i := 0; i < 3; i++ {
		opts := logOpts(repo, "rt-"+itoa(i)+".png", "roundtrip-"+itoa(i), t)
		if _, err := LogStep(store, run, opts); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := ledger.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for i, entry := range entries {
		if entry.Step != i+1 {
			t.Fatalf("entry %d: expected step %d, got %d", i, i+1, entry.Step)
		}
		if entry.Observation != testObservation {
			t.Fatalf("entry %d: expected observation, got %q", i, entry.Observation)
		}
		if entry.Comparison != testComparison {
			t.Fatalf("entry %d: expected comparison, got %q", i, entry.Comparison)
		}
	}
}

func TestScreenshotLogLedgerLoadForScenario(t *testing.T) {
	store, run, repo := setupLogFixture(t)

	// Log to two scenarios.
	opts1 := logOpts(repo, "hp.png", "happy-log", t)
	if _, err := LogStep(store, run, opts1); err != nil {
		t.Fatal(err)
	}

	opts2 := logOpts(repo, "fp.png", "failure-log", t)
	opts2.ScenarioID = "failure-path"
	if _, err := LogStep(store, run, opts2); err != nil {
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

func TestScreenshotLogLedgerNextStep(t *testing.T) {
	store, run, repo := setupLogFixture(t)
	ledger := store.ScreenshotLogLedger(run)

	// Empty ledger starts at step 1.
	next, err := ledger.NextStep("happy-path", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if next != 1 {
		t.Fatalf("expected next step 1, got %d", next)
	}

	// After logging one step, next is 2.
	opts := logOpts(repo, "ns.png", "nextstep", t)
	if _, err := LogStep(store, run, opts); err != nil {
		t.Fatal(err)
	}
	next, err = ledger.NextStep("happy-path", "log-session-1")
	if err != nil {
		t.Fatal(err)
	}
	if next != 2 {
		t.Fatalf("expected next step 2, got %d", next)
	}
}

func TestScreenshotLogPath(t *testing.T) {
	store, run, _ := setupLogFixture(t)

	path := store.ScreenshotLogPath(run)
	if !strings.HasSuffix(path, "screenshot-log.jsonl") {
		t.Fatalf("expected path to end with screenshot-log.jsonl, got %s", path)
	}
}

func TestLogStepBadScreenshot(t *testing.T) {
	store, run, _ := setupLogFixture(t)
	_, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "s",
		Surface:        SurfaceBrowser,
		ScreenshotPath: "/nonexistent/path/screenshot.png",
		Action:         testAction,
		Observation:    testObservation,
		Comparison:     testComparison,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent screenshot")
	}
	if !strings.Contains(err.Error(), "copy screenshot") {
		t.Fatalf("expected copy error, got: %v", err)
	}
}

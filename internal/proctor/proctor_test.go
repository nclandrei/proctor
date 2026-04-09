package proctor

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCreateRunWritesExpectedFiles(t *testing.T) {
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

	for _, path := range []string{
		filepath.Join(store.RunDir(run), "run.json"),
		filepath.Join(store.RunDir(run), "contract.md"),
		filepath.Join(store.RunDir(run), "report.html"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}

	contract, err := os.ReadFile(filepath.Join(store.RunDir(run), "contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	contractText := string(contract)
	if !strings.Contains(contractText, "## Edge Case Coverage") {
		t.Fatalf("expected contract to include edge case coverage section, got:\n%s", contractText)
	}
	if !strings.Contains(contractText, "empty or missing input") || !strings.Contains(contractText, "covered elsewhere") {
		t.Fatalf("expected contract to include N/A reason for empty input category, got:\n%s", contractText)
	}
	if !strings.Contains(contractText, "bad email shows validation") {
		t.Fatalf("expected contract to include edge-case scenario labels, got:\n%s", contractText)
	}

	report, err := os.ReadFile(filepath.Join(store.RunDir(run), "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	reportText := string(report)
	if !strings.Contains(reportText, "Verification Report") || !strings.Contains(reportText, "class=\"meta-list\"") {
		t.Fatalf("expected report html to include the metadata list, got:\n%s", reportText)
	}
	if !strings.Contains(reportText, "summary-line") || !strings.Contains(reportText, "--bg: #ffffff") {
		t.Fatalf("expected report html to include the summary line, got:\n%s", reportText)
	}
	if !strings.Contains(reportText, `class="summary-line"`) {
		t.Fatalf("expected report html to include summary line, got:\n%s", reportText)
	}
	if !strings.Contains(reportText, "scenarios:") || !strings.Contains(reportText, "passed") {
		t.Fatalf("expected report html to include inline scenario summary, got:\n%s", reportText)
	}
	if !strings.Contains(reportText, `<meta name="color-scheme" content="light">`) {
		t.Fatalf("expected report html to set color-scheme, got:\n%s", reportText)
	}
}

func TestStaleScreenshotIsRejected(t *testing.T) {
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
	// Create a screenshot and set its mtime to 2 hours ago.
	staleShot := writeScreenshotFixture(t, repo, "stale.png", "stale-image")
	staleTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(staleShot, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	filePreNote(t, store, run, "happy-path", "browser-1")
	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-stale": staleShot,
		},
		PassAssertions:   []string{"console_errors = 0"},
		MaxScreenshotAge: 30 * time.Minute,
	})
	if err == nil || !strings.Contains(err.Error(), "too old") {
		t.Fatalf("expected stale screenshot rejection, got %v", err)
	}
}

func TestCrossScenarioDuplicateScreenshotIsRejected(t *testing.T) {
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
	sharedShot := writeScreenshotFixture(t, repo, "shared.png", "identical-screenshot-content")

	filePreNote(t, store, run, "happy-path", "browser-1")
	filePreNote(t, store, run, "failure-path", "browser-1")
	// First recording succeeds.
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": sharedShot,
		},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatal(err)
	}

	// Second recording with the same screenshot content for a different scenario is rejected.
	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "failure-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-failure": sharedShot,
		},
		PassAssertions: []string{"console_errors = 0"},
	})
	if err == nil || !strings.Contains(err.Error(), "identical content") {
		t.Fatalf("expected duplicate screenshot rejection, got %v", err)
	}
}

func TestSameScenarioReRecordingAllowed(t *testing.T) {
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
	shot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image-content")

	filePreNote(t, store, run, "happy-path", "browser-1")
	// First recording.
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-first": shot,
		},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatal(err)
	}

	// Second recording for the same scenario should be allowed (append-only).
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-second": shot,
		},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("expected same-scenario re-recording to be allowed, got %v", err)
	}
}

func TestFreshScreenshotIsAccepted(t *testing.T) {
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
	freshShot := writeScreenshotFixture(t, repo, "fresh.png", "fresh-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-fresh": freshShot,
		},
		PassAssertions:   []string{"console_errors = 0"},
		MaxScreenshotAge: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("expected fresh screenshot to be accepted, got %v", err)
	}
}

func TestReportEmbedsScreenshotPreviews(t *testing.T) {
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
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{
			"console_errors = 0",
		},
	}); err != nil {
		t.Fatal(err)
	}

	reportHTML, err := os.ReadFile(filepath.Join(store.RunDir(run), "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(reportHTML)
	if !strings.Contains(text, "data:image/png;base64,") {
		t.Fatalf("expected report html to embed screenshot previews, got:\n%s", text)
	}
	if !strings.Contains(text, "Click to enlarge") {
		t.Fatalf("expected report html to include the compact preview affordance, got:\n%s", text)
	}
	if !strings.Contains(text, `class="lightbox"`) {
		t.Fatalf("expected report html to include inline enlarge markup, got:\n%s", text)
	}
	if strings.Contains(text, `<img src="artifacts/`) {
		t.Fatalf("expected report html image tags to use embedded data urls, got:\n%s", text)
	}
}

func TestReportDisplaysEvidenceTimestamps(t *testing.T) {
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
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
		},
		PassAssertions: []string{
			"console_errors = 0",
		},
	}); err != nil {
		t.Fatal(err)
	}

	reportHTML, err := os.ReadFile(filepath.Join(store.RunDir(run), "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(reportHTML)
	if !strings.Contains(text, `class="evidence-timestamp"`) {
		t.Fatalf("expected report html to include evidence timestamps, got:\n%s", text)
	}
	if !strings.Contains(text, "Captured:") {
		t.Fatalf("expected report html to include 'Captured:' timestamp label, got:\n%s", text)
	}
	if !strings.Contains(text, "UTC") {
		t.Fatalf("expected report html to include UTC timestamp, got:\n%s", text)
	}
}

func TestReportEmbedsTranscriptAsCollapsedLog(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "curl-1")
	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "happy-path",
		Command: []string{
			"sh",
			"-c",
			`printf 'HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n<status> ready & waiting\nsecond line\n'`,
		},
		PassAssertions: []string{
			"status = 200",
			"body contains ready",
		},
	}); err != nil {
		t.Fatal(err)
	}

	reportHTML, err := os.ReadFile(filepath.Join(store.RunDir(run), "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(reportHTML)
	if !strings.Contains(text, `class="transcript"`) {
		t.Fatalf("expected report html to include collapsible transcript markup, got:\n%s", text)
	}
	if !strings.Contains(text, "Transcript") {
		t.Fatalf("expected report html to describe the inline transcript, got:\n%s", text)
	}
	if !strings.Contains(text, `class="log"`) {
		t.Fatalf("expected report html to include the embedded transcript body, got:\n%s", text)
	}
	if !strings.Contains(text, "&lt;status&gt; ready &amp; waiting") {
		t.Fatalf("expected embedded transcript content to be HTML-escaped, got:\n%s", text)
	}
}

func TestCreateRunRequiresExplicitCurlModeAndReasoning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	_, err = CreateRun(store, repo, StartOptions{
		Feature:     "missing curl mode",
		BrowserURL:  "http://127.0.0.1:3000",
		HappyPath:   "happy",
		FailurePath: "failure",
	})
	if err == nil || !strings.Contains(err.Error(), "--curl must be one of required, scenario, or skip") {
		t.Fatalf("expected explicit curl mode validation, got %v", err)
	}

	_, err = CreateRun(store, repo, StartOptions{
		Feature:        "skip without reason",
		BrowserURL:     "http://127.0.0.1:3000",
		CurlMode:       "skip",
		HappyPath:      "happy",
		FailurePath:    "failure",
		EdgeCaseInputs: []string{"validation and malformed input=N/A: covered elsewhere"},
	})
	if err == nil || !strings.Contains(err.Error(), "--curl-skip-reason is required when --curl skip") {
		t.Fatalf("expected curl skip reason validation, got %v", err)
	}
}

func TestCreateRunRequiresExplicitEdgeCaseCoverage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	removeCategory := func(inputs []string, category string) []string {
		var filtered []string
		for _, input := range inputs {
			key, _, ok := parseEdgeCaseInput(input)
			if ok && strings.EqualFold(key, category) {
				continue
			}
			filtered = append(filtered, input)
		}
		return filtered
	}
	replaceCategory := func(inputs []string, category, value string) []string {
		replaced := false
		for idx, input := range inputs {
			key, _, ok := parseEdgeCaseInput(input)
			if ok && strings.EqualFold(key, category) {
				inputs[idx] = category + "=" + value
				replaced = true
				break
			}
		}
		if !replaced {
			inputs = append(inputs, category+"="+value)
		}
		return inputs
	}

	testCases := []struct {
		name      string
		opts      StartOptions
		wantError string
	}{
		{
			name: "missing web category",
			opts: StartOptions{
				Feature:        "edge-case validation",
				BrowserURL:     "http://127.0.0.1:3000/login",
				CurlMode:       "skip",
				CurlSkipReason: "No backend coverage needed for this test",
				HappyPath:      "happy",
				FailurePath:    "failure",
				EdgeCaseInputs: removeCategory(allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformWeb)), "any feature-specific risks"),
			},
			wantError: `missing required edge-case coverage for "any feature-specific risks"`,
		},
		{
			name: "missing ios-specific category",
			opts: StartOptions{
				Platform:       PlatformIOS,
				Feature:        "ios edge-case validation",
				IOSScheme:      "Pagena",
				IOSBundleID:    "com.example.pagena",
				CurlMode:       "skip",
				CurlSkipReason: "UI-only verification for this test",
				HappyPath:      "happy",
				FailurePath:    "failure",
				EdgeCaseInputs: removeCategory(allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformIOS)), "device traits, orientation, and layout"),
			},
			wantError: `missing required edge-case coverage for "device traits, orientation, and layout"`,
		},
		{
			name: "na without reason",
			opts: StartOptions{
				Feature:        "edge-case validation",
				BrowserURL:     "http://127.0.0.1:3000/login",
				CurlMode:       "skip",
				CurlSkipReason: "No backend coverage needed for this test",
				HappyPath:      "happy",
				FailurePath:    "failure",
				EdgeCaseInputs: replaceCategory(allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformWeb)), "empty or missing input", "N/A"),
			},
			wantError: `edge-case "empty or missing input" must use "N/A: reason"`,
		},
		{
			name: "scenario list without concrete scenarios",
			opts: StartOptions{
				Feature:        "edge-case validation",
				BrowserURL:     "http://127.0.0.1:3000/login",
				CurlMode:       "skip",
				CurlSkipReason: "No backend coverage needed for this test",
				HappyPath:      "happy",
				FailurePath:    "failure",
				EdgeCaseInputs: replaceCategory(allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformWeb)), "retry or double-submit", " ; "),
			},
			wantError: `edge-case "retry or double-submit" must list one or more concrete scenarios or use "N/A: reason"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateRun(store, repo, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
			}
		})
	}
}

func TestCreateRunSupportsScenarioLevelCurlRequirements(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	run, err := CreateRun(store, repo, StartOptions{
		Feature:    "scenario curl plan",
		BrowserURL: "http://127.0.0.1:3000/login",
		CurlMode:   "scenario",
		CurlEndpoints: []string{
			"happy-path=POST /api/login",
			"already signed-in users are redirected away from /login=GET /api/session",
		},
		HappyPath:   "valid login goes to dashboard",
		FailurePath: "invalid password shows error",
		EdgeCaseInputs: []string{
			"validation and malformed input=bad email shows validation",
			"empty or missing input=n/a: covered elsewhere",
			"retry or double-submit=double submit is ignored",
			"loading, latency, and race conditions=n/a: not relevant",
			"network or server failure=n/a: not relevant",
			"auth and session state=already signed-in users are redirected away from /login",
			"refresh, back-navigation, and state persistence=n/a: not relevant",
			"mobile or responsive behavior=n/a: not relevant",
			"accessibility and keyboard behavior=n/a: not relevant",
			"any feature-specific risks=n/a: not relevant",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	scenarios := map[string]Scenario{}
	for _, scenario := range run.Scenarios {
		scenarios[scenario.ID] = scenario
	}

	if !scenarios["happy-path"].CurlRequired {
		t.Fatalf("expected happy-path to require curl")
	}
	if got := scenarios["happy-path"].CurlEndpoints; len(got) != 1 || got[0] != "POST /api/login" {
		t.Fatalf("expected happy-path curl endpoints, got %#v", got)
	}
	if !scenarios["auth-and-session-state-already-signed-in-users-are-redirected-away-from-login"].CurlRequired {
		t.Fatalf("expected auth/session edge case to require curl")
	}
	if got := scenarios["auth-and-session-state-already-signed-in-users-are-redirected-away-from-login"].CurlEndpoints; len(got) != 1 || got[0] != "GET /api/session" {
		t.Fatalf("expected auth/session curl endpoints, got %#v", got)
	}
	if scenarios["failure-path"].CurlRequired {
		t.Fatalf("expected failure-path to remain browser-only when not listed in the curl plan")
	}
	if scenarios["validation-and-malformed-input-bad-email-shows-validation"].CurlRequired {
		t.Fatalf("expected unrelated edge case to remain browser-only when not listed in the curl plan")
	}
}

func TestEvaluateRequiresCurlForScenarioModeEdgeCases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	run, err := CreateRun(store, repo, StartOptions{
		Feature:    "scenario curl gating",
		BrowserURL: "http://127.0.0.1:3000/login",
		CurlMode:   "scenario",
		CurlEndpoints: []string{
			"already signed-in users are redirected away from /login=GET /api/session",
		},
		HappyPath:   "valid login goes to dashboard",
		FailurePath: "invalid password shows error",
		EdgeCaseInputs: []string{
			"validation and malformed input=bad email shows validation",
			"empty or missing input=n/a: covered elsewhere",
			"retry or double-submit=double submit is ignored",
			"loading, latency, and race conditions=n/a: not relevant",
			"network or server failure=n/a: not relevant",
			"auth and session state=already signed-in users are redirected away from /login",
			"refresh, back-navigation, and state persistence=n/a: not relevant",
			"mobile or responsive behavior=n/a: not relevant",
			"accessibility and keyboard behavior=n/a: not relevant",
			"any feature-specific risks=n/a: not relevant",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	filePreNotesForAll(t, store, run, "browser-1", testPreNoteText)
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/dashboard", 0, 0, 0, 0))
	for i, scenario := range run.Scenarios {
		desktopShot := writeScreenshotFixture(t, repo, fmt.Sprintf("desktop-%d.png", i), fmt.Sprintf("desktop-image-%s", scenario.ID))
		if err := RecordBrowser(store, run, BrowserRecordOptions{
			ScenarioID: scenario.ID,
			SessionID:  "browser-1",
			ReportPath: report,
			Screenshots: map[string]string{
				"desktop-success": desktopShot,
			},
			PassAssertions: []string{
				"console_errors = 0",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}

	var authScenario ScenarioEvaluation
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.ID == "auth-and-session-state-already-signed-in-users-are-redirected-away-from-login" {
			authScenario = item
			break
		}
	}
	if authScenario.CurlOK {
		t.Fatalf("expected auth/session scenario to fail without curl evidence")
	}
	if !containsSubstring(authScenario.CurlIssues, "missing curl evidence") {
		t.Fatalf("expected missing curl evidence issue, got %#v", authScenario.CurlIssues)
	}
	if eval.Complete {
		t.Fatalf("expected run to remain incomplete while required edge-case curl is missing")
	}
}

func TestLoadRunNormalizesLegacyRunWideCurlRequirement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	repoSlug, err := RepoSlug(repo)
	if err != nil {
		t.Fatal(err)
	}
	run := Run{
		ID:            "run-legacy",
		RepoSlug:      repoSlug,
		RepoRoot:      repo,
		Feature:       "legacy curl",
		BrowserURL:    "http://127.0.0.1:3000/login",
		CurlRequired:  true,
		CurlEndpoints: []string{"POST /api/login"},
		HappyPath:     Scenario{ID: "happy-path", Label: "happy", Kind: "happy-path", BrowserRequired: true},
		FailurePath:   Scenario{ID: "failure-path", Label: "failure", Kind: "failure-path", BrowserRequired: true},
		Scenarios:     []Scenario{{ID: "happy-path", Label: "happy", Kind: "happy-path", BrowserRequired: true}, {ID: "failure-path", Label: "failure", Kind: "failure-path", BrowserRequired: true}},
		Status:        StatusInProgress,
		CreatedAt:     mustParseTime(t, "2026-03-16T10:00:00Z"),
		UpdatedAt:     mustParseTime(t, "2026-03-16T10:00:00Z"),
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveRun(run); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadRun(repo)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.CurlMode != CurlModeRequired {
		t.Fatalf("expected legacy run to normalize to required mode, got %q", loaded.CurlMode)
	}
	for _, scenarioID := range []string{"happy-path", "failure-path"} {
		scenario, ok := findScenario(loaded, scenarioID)
		if !ok {
			t.Fatalf("expected scenario %s in loaded run", scenarioID)
		}
		if !scenario.CurlRequired {
			t.Fatalf("expected scenario %s to inherit legacy curl requirement", scenarioID)
		}
		if got := scenario.CurlEndpoints; len(got) != 1 || got[0] != "POST /api/login" {
			t.Fatalf("expected scenario %s to inherit legacy endpoint, got %#v", scenarioID, got)
		}
	}
}

func TestRecordBrowserEvaluatesStructuredAssertions(t *testing.T) {
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
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{
			"final_url contains /dashboard",
			"console_errors = 0",
			"failed_requests = 0",
			"desktop_screenshot = true",
			"mobile_screenshot = true",
		},
	}); err != nil {
		t.Fatal(err)
	}

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(evidence))
	}
	for _, assertion := range evidence[0].Assertions {
		if assertion.Result != AssertionPass {
			t.Fatalf("expected passing assertion, got %#v", assertion)
		}
	}
}

func TestBrowserWarningsAreRecordedButNotImplicitlyBlocking(t *testing.T) {
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

	report := writeFixture(t, repo, "report.json", sampleBrowserReportWithWarnings("http://127.0.0.1:3000/dashboard", 0, 2, 0, 0, 0))
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
		},
		PassAssertions: []string{
			"final_url contains /dashboard",
		},
	}); err != nil {
		t.Fatal(err)
	}

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	if got := evidence[0].Browser.Desktop.ConsoleWarnings; got != 2 {
		t.Fatalf("expected browser evidence to record console warnings, got %d", got)
	}
	for _, assertion := range evidence[0].Assertions {
		if assertion.Description == "console_warnings = 0" || assertion.Description == "desktop.console_warnings = 0" {
			t.Fatalf("expected default browser assertions to leave warnings advisory, got %#v", assertion)
		}
	}

	verifyAllScenarios(t, store, run, testObservationNotes)

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if !eval.ScenarioEvaluations[0].BrowserOK {
		t.Fatalf("expected warnings alone to stay non-blocking by default, got %#v", eval.ScenarioEvaluations[0].BrowserIssues)
	}
}

func TestBrowserWarningsCanBeMadeBlockingWithExplicitAssertion(t *testing.T) {
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

	report := writeFixture(t, repo, "report.json", sampleBrowserReportWithWarnings("http://127.0.0.1:3000/dashboard", 0, 2, 0, 0, 0))
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	// Record with an intentionally failing assertion; error expected.
	_ = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
		},
		PassAssertions: []string{
			"final_url contains /dashboard",
			"console_warnings = 0",
		},
	})

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].BrowserOK {
		t.Fatalf("expected explicit warning assertion to block the scenario")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].BrowserIssues, "assertion failed: console_warnings = 0") {
		t.Fatalf("expected warning assertion failure, got %#v", eval.ScenarioEvaluations[0].BrowserIssues)
	}
}

func TestDonePassesWhenRequiredEvidenceExists(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/login" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("mode") == "failure" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"invalid"}`)
			return
		}

		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	successReport := writeFixture(t, repo, "success-report.json", sampleBrowserReport("http://127.0.0.1:3000/dashboard", 0, 0, 0, 0))
	failureReport := writeFixture(t, repo, "failure-report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 1))

	scenarioCounter := 0
	uniqueShot := func(label string) string {
		scenarioCounter++
		return writeScreenshotFixture(t, repo, fmt.Sprintf("%s-%d.png", label, scenarioCounter), fmt.Sprintf("%s-image-%d", label, scenarioCounter))
	}

	filePreNotesForAll(t, store, run, "browser-1", testPreNoteText)
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: successReport,
		Screenshots: map[string]string{
			"desktop-success": uniqueShot("desktop"),
			"mobile-success":  uniqueShot("mobile"),
		},
		PassAssertions: []string{
			"final_url contains /dashboard",
			"console_errors = 0",
			"failed_requests = 0",
			"mobile_screenshot = true",
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "failure-path",
		SessionID:  "browser-1",
		ReportPath: failureReport,
		Screenshots: map[string]string{
			"desktop-failure": uniqueShot("desktop"),
		},
		PassAssertions: []string{
			"final_url contains /login",
			"http_errors = 1",
		},
	}); err != nil {
		t.Fatal(err)
	}

	for _, scenario := range run.Scenarios {
		if scenario.Kind != "edge-case" {
			continue
		}
		screenshots := map[string]string{
			"desktop-edge": uniqueShot("desktop"),
		}
		if scenario.Category == "mobile or responsive behavior" {
			screenshots["mobile-edge"] = uniqueShot("mobile")
		}
		if err := RecordBrowser(store, run, BrowserRecordOptions{
			ScenarioID:  scenario.ID,
			SessionID:   "browser-1",
			ReportPath:  successReport,
			Screenshots: screenshots,
			PassAssertions: []string{
				"console_errors = 0",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "happy-path",
		Command:    curlHelperCommand("http", "POST", server.URL+"/api/login"),
		PassAssertions: []string{
			"status = 200",
			"header.content-type contains application/json",
			"body contains ok",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "failure-path",
		Command:    curlHelperCommand("http", "POST", server.URL+"/api/login?mode=failure"),
		PassAssertions: []string{
			"status = 401",
			"body contains invalid",
		},
	}); err != nil {
		t.Fatal(err)
	}

	logStepsForAll(t, store, run, repo, "browser-1")
	verifyAllScenarios(t, store, run, testObservationNotes)

	eval, err := CompleteRun(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if !eval.Complete {
		t.Fatalf("expected run to pass, got %#v", eval)
	}

	contract, err := os.ReadFile(filepath.Join(store.RunDir(run), "contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contract), "desktop-success") {
		t.Fatalf("expected contract to reference screenshot artifact, got:\n%s", contract)
	}
	if !strings.Contains(string(contract), "Desktop final URL: `http://127.0.0.1:3000/dashboard`") {
		t.Fatalf("expected contract to include browser final URL summary, got:\n%s", contract)
	}
	if !strings.Contains(string(contract), "Session: `browser-1`") {
		t.Fatalf("expected contract to include browser session summary, got:\n%s", contract)
	}
	if !strings.Contains(string(contract), "Response status: `200`") {
		t.Fatalf("expected contract to include curl response summary, got:\n%s", contract)
	}
}

func TestDoneFailsWhenBrowserAssertionFails(t *testing.T) {
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

	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/dashboard", 1, 0, 0, 0))
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	// Record with an intentionally failing assertion; error expected.
	_ = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{
			"console_errors = 0",
		},
	})

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.Complete {
		t.Fatalf("expected evaluation to remain incomplete")
	}
}

func TestEvaluateRequiresGlobalMobileScreenshotCoverage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, desktopCoverageOnlyStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "desktop-only-report.json", sampleDesktopOnlyBrowserReport("http://127.0.0.1:3000/dashboard"))

	filePreNotesForAll(t, store, run, "browser-1", testPreNoteText)
	for i, scenarioID := range []string{"happy-path", "failure-path"} {
		desktopShot := writeScreenshotFixture(t, repo, fmt.Sprintf("desktop-%d.png", i), fmt.Sprintf("desktop-image-%s", scenarioID))
		if err := RecordBrowser(store, run, BrowserRecordOptions{
			ScenarioID: scenarioID,
			SessionID:  "browser-1",
			ReportPath: report,
			Screenshots: map[string]string{
				"desktop-success": desktopShot,
			},
			PassAssertions: []string{
				"final_url contains /dashboard",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	verifyAllScenarios(t, store, run, testObservationNotes)

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.Complete {
		t.Fatalf("expected evaluation to fail without a mobile screenshot")
	}
	for _, scenarioEval := range eval.ScenarioEvaluations {
		if !scenarioEval.BrowserOK {
			t.Fatalf("expected scenario %s to pass browser validation so the remaining failure is global coverage, got %#v", scenarioEval.Scenario.ID, scenarioEval.BrowserIssues)
		}
	}
	if !containsSubstring(eval.GlobalMissing, "at least one mobile screenshot") {
		t.Fatalf("expected global missing coverage to require a mobile screenshot, got %#v", eval.GlobalMissing)
	}
}

func TestBrowserImplicitHealthChecksFailWhenIssuesAreUnaccountedFor(t *testing.T) {
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

	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/dashboard", 0, 0, 2, 0))
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	// Record with implicit health checks that will fail; error expected.
	_ = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{
			"final_url contains /dashboard",
		},
	})

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	foundImplicitFailure := false
	for _, assertion := range evidence[0].Assertions {
		if assertion.Description == "failed_requests = 0" && assertion.Result == AssertionFail {
			foundImplicitFailure = true
		}
	}
	if !foundImplicitFailure {
		t.Fatalf("expected implicit failed_requests assertion to fail")
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].BrowserOK {
		t.Fatalf("expected browser evidence to fail implicit health checks")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].BrowserIssues, "assertion failed: failed_requests = 0") {
		t.Fatalf("expected browser issues to include failed assertion detail, got %#v", eval.ScenarioEvaluations[0].BrowserIssues)
	}
}

func TestExplicitDesktopIssueAssertionOverridesImplicitZeroCheck(t *testing.T) {
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

	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 1))
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "failure-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "failure-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-failure": desktopShot,
			"mobile-failure":  mobileShot,
		},
		PassAssertions: []string{
			"desktop.http_errors = 1",
			"mobile.http_errors = 0",
		},
	}); err != nil {
		t.Fatal(err)
	}

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	for _, assertion := range evidence[0].Assertions {
		if assertion.Description == "http_errors = 0" && assertion.Result == AssertionFail {
			t.Fatalf("unexpected implicit desktop http_errors failure: %#v", assertion)
		}
	}
}

func TestCurlEvaluationIncludesFailedAssertionDetails(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "curl-1")
	// Record with an intentionally failing assertion; error expected.
	_ = RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "happy-path",
		Command:    []string{"/bin/sh", "-lc", "printf 'HTTP/1.1 500 Internal Server Error\\nContent-Type: application/json\\n\\n{\"error\":\"boom\"}'"},
		PassAssertions: []string{
			"status = 200",
		},
	})

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].CurlOK {
		t.Fatalf("expected curl evidence to fail")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].CurlIssues, "assertion failed: status = 200") {
		t.Fatalf("expected curl issues to include failed assertion detail, got %#v", eval.ScenarioEvaluations[0].CurlIssues)
	}
}

func TestEvaluateRequiresRealHTTPResponseAndMatchingCurlEndpointContract(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/session":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"session":"active"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/login":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	filePreNote(t, store, run, "happy-path", "curl-1")
	filePreNote(t, store, run, "failure-path", "curl-1")
	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "happy-path",
		Command:    curlHelperCommand("http", "GET", server.URL+"/api/session"),
		PassAssertions: []string{
			"status = 200",
			"body contains active",
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "failure-path",
		Command:    curlHelperCommand("plain", "not http"),
		PassAssertions: []string{
			"exit_code = 0",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}

	happyPathEval := findScenarioEvaluation(t, eval, "happy-path")
	if happyPathEval.CurlOK {
		t.Fatalf("expected happy-path curl evidence to fail when the wrapped request misses the declared endpoint contract")
	}
	if !containsSubstring(happyPathEval.CurlIssues, "endpoint") {
		t.Fatalf("expected happy-path curl issues to mention the endpoint contract, got %#v", happyPathEval.CurlIssues)
	}

	failurePathEval := findScenarioEvaluation(t, eval, "failure-path")
	if failurePathEval.CurlOK {
		t.Fatalf("expected failure-path curl evidence to fail without a real HTTP response")
	}
	if !containsSubstring(failurePathEval.CurlIssues, "HTTP response") {
		t.Fatalf("expected failure-path curl issues to mention the missing HTTP response, got %#v", failurePathEval.CurlIssues)
	}
}

func TestMobileScreenshotRequiresMobileReportResults(t *testing.T) {
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

	report := writeFixture(t, repo, "desktop-only-report.json", `{
  "desktop": {
    "title": "Login",
    "finalUrl": "http://127.0.0.1:3000/dashboard",
    "issues": {
      "consoleErrors": 0,
      "consoleWarnings": 0,
      "pageErrors": 0,
      "failedRequests": 0,
      "httpErrors": 0
    }
  }
}`)
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{
			"final_url contains /dashboard",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].BrowserOK {
		t.Fatalf("expected browser evidence to fail without mobile report results")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].BrowserIssues, "browser report is missing mobile results for attached mobile screenshot") {
		t.Fatalf("expected mobile report issue, got %#v", eval.ScenarioEvaluations[0].BrowserIssues)
	}
}

func TestBrowserReportRequiresDesktopFinalURL(t *testing.T) {
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

	report := writeFixture(t, repo, "missing-final-url-report.json", `{
  "desktop": {
    "title": "Login",
    "finalUrl": "",
    "issues": {
      "consoleErrors": 0,
      "consoleWarnings": 0,
      "pageErrors": 0,
      "failedRequests": 0,
      "httpErrors": 0
    }
  },
  "mobile": {
    "title": "Login",
    "finalUrl": "http://127.0.0.1:3000/dashboard",
    "issues": {
      "consoleErrors": 0,
      "consoleWarnings": 0,
      "pageErrors": 0,
      "failedRequests": 0,
      "httpErrors": 0
    }
  }
}`)
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "happy-path", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{
			"mobile.final_url contains /dashboard",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].BrowserOK {
		t.Fatalf("expected browser evidence to fail without a desktop final URL")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].BrowserIssues, "browser report is missing a desktop final URL") {
		t.Fatalf("expected desktop final URL issue, got %#v", eval.ScenarioEvaluations[0].BrowserIssues)
	}
	if containsSubstring(eval.GlobalMissing, "desktop screenshot") || containsSubstring(eval.GlobalMissing, "mobile screenshot") {
		t.Fatalf("expected global screenshot coverage to count attached screenshots even when the scenario fails, got %#v", eval.GlobalMissing)
	}
}

func TestMobileResponsiveScenarioRequiresMobileScreenshot(t *testing.T) {
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
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")

	filePreNote(t, store, run, "mobile-or-responsive-behavior-layout-remains-usable-on-mobile", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "mobile-or-responsive-behavior-layout-remains-usable-on-mobile",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
		},
		PassAssertions: []string{
			"final_url contains /dashboard",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	var scenarioEval ScenarioEvaluation
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.ID == "mobile-or-responsive-behavior-layout-remains-usable-on-mobile" {
			scenarioEval = item
			break
		}
	}
	if scenarioEval.BrowserOK {
		t.Fatalf("expected mobile-responsive scenario to fail without a mobile screenshot")
	}
	if !containsSubstring(scenarioEval.BrowserIssues, "mobile or responsive behavior scenarios require a mobile screenshot") {
		t.Fatalf("expected mobile screenshot requirement, got %#v", scenarioEval.BrowserIssues)
	}
}

func TestMobileResponsiveScenarioPassesWithMobileProof(t *testing.T) {
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
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-image")

	filePreNote(t, store, run, "mobile-or-responsive-behavior-layout-remains-usable-on-mobile", "browser-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "mobile-or-responsive-behavior-layout-remains-usable-on-mobile",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{
			"mobile.final_url contains /dashboard",
		},
	}); err != nil {
		t.Fatal(err)
	}

	verifyAllScenarios(t, store, run, testObservationNotes)

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	var scenarioEval ScenarioEvaluation
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.ID == "mobile-or-responsive-behavior-layout-remains-usable-on-mobile" {
			scenarioEval = item
			break
		}
	}
	if !scenarioEval.BrowserOK {
		t.Fatalf("expected mobile-responsive scenario to pass with mobile proof, got %#v", scenarioEval.BrowserIssues)
	}
}

func sampleStartOptions() StartOptions {
	return StartOptions{
		Feature:       "auth flow",
		BrowserURL:    "http://127.0.0.1:3000/login",
		CurlMode:      "required",
		CurlEndpoints: []string{"POST /api/login"},
		HappyPath:     "valid login goes to dashboard",
		FailurePath:   "invalid password shows error",
		EdgeCaseInputs: []string{
			"validation and malformed input=bad email shows validation",
			"empty or missing input=n/a: covered elsewhere",
			"retry or double-submit=double submit is ignored",
			"loading, latency, and race conditions=n/a: not relevant",
			"network or server failure=n/a: not relevant",
			"auth and session state=n/a: not relevant",
			"refresh, back-navigation, and state persistence=n/a: not relevant",
			"mobile or responsive behavior=layout remains usable on mobile",
			"accessibility and keyboard behavior=n/a: not relevant",
			"any feature-specific risks=n/a: not relevant",
		},
	}
}

func desktopCoverageOnlyStartOptions() StartOptions {
	inputs := make([]string, 0, len(EdgeCaseCategories))
	for _, category := range EdgeCaseCategories {
		inputs = append(inputs, category+"=N/A: covered by this test")
	}
	return StartOptions{
		Feature:        "desktop-only coverage test",
		BrowserURL:     "http://127.0.0.1:3000/dashboard",
		CurlMode:       "skip",
		CurlSkipReason: "No backend coverage needed for this test",
		HappyPath:      "happy path stays on dashboard",
		FailurePath:    "failure path also stays on dashboard for this test",
		EdgeCaseInputs: inputs,
	}
}

func findScenarioEvaluation(t *testing.T, eval Evaluation, scenarioID string) ScenarioEvaluation {
	t.Helper()
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.ID == scenarioID {
			return item
		}
	}
	t.Fatalf("missing scenario evaluation for %s", scenarioID)
	return ScenarioEvaluation{}
}

func curlHelperCommand(mode string, args ...string) []string {
	command := []string{os.Args[0], "-test.run=TestCurlRecordHelperProcess", "--", mode}
	command = append(command, args...)
	return command
}

func TestCurlRecordHelperProcess(t *testing.T) {
	if !strings.Contains(strings.Join(os.Args, " "), "TestCurlRecordHelperProcess") {
		return
	}

	args := os.Args
	marker := -1
	for idx, arg := range args {
		if arg == "--" {
			marker = idx
			break
		}
	}
	if marker == -1 || marker+1 >= len(args) {
		fmt.Fprintln(os.Stderr, "missing helper args")
		os.Exit(2)
	}

	mode := args[marker+1]
	switch mode {
	case "http":
		if marker+4 > len(args) {
			fmt.Fprintln(os.Stderr, "usage: http METHOD URL")
			os.Exit(2)
		}
		method := args[marker+2]
		url := args[marker+3]
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		fmt.Printf("HTTP/1.1 %d %s\r\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		for name, values := range resp.Header {
			for _, value := range values {
				fmt.Printf("%s: %s\r\n", name, value)
			}
		}
		fmt.Print("\r\n")
		fmt.Print(string(body))
	case "plain":
		if marker+2 >= len(args) {
			fmt.Fprintln(os.Stderr, "usage: plain TEXT")
			os.Exit(2)
		}
		fmt.Print(args[marker+2])
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		os.Exit(2)
	}

	os.Exit(0)
}

func sampleBrowserReport(finalURL string, consoleErrors, pageErrors, failedRequests, httpErrors int) string {
	return sampleBrowserReportWithWarnings(finalURL, consoleErrors, 0, pageErrors, failedRequests, httpErrors)
}

func sampleBrowserReportWithWarnings(finalURL string, consoleErrors, consoleWarnings, pageErrors, failedRequests, httpErrors int) string {
	return `{
  "desktop": {
    "title": "Login",
    "finalUrl": "` + finalURL + `",
    "issues": {
      "consoleErrors": ` + itoa(consoleErrors) + `,
      "consoleWarnings": ` + itoa(consoleWarnings) + `,
      "pageErrors": ` + itoa(pageErrors) + `,
      "failedRequests": ` + itoa(failedRequests) + `,
      "httpErrors": ` + itoa(httpErrors) + `
    }
  },
  "mobile": {
    "title": "Login",
    "finalUrl": "` + finalURL + `",
    "issues": {
      "consoleErrors": 0,
      "consoleWarnings": 0,
      "pageErrors": 0,
      "failedRequests": 0,
      "httpErrors": 0
    }
  }
}`
}

func sampleDesktopOnlyBrowserReport(finalURL string) string {
	return `{
  "desktop": {
    "title": "Dashboard",
    "finalUrl": "` + finalURL + `",
    "issues": {
      "consoleErrors": 0,
      "consoleWarnings": 0,
      "pageErrors": 0,
      "failedRequests": 0,
      "httpErrors": 0
    }
  }
}`
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func initGitRepo(t *testing.T, repo, remote string) {
	t.Helper()
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@example.com")
	mustRun(t, repo, "git", "config", "user.name", "Test User")
	mustRun(t, repo, "git", "remote", "add", "origin", remote)
}

func mustRun(t *testing.T, dir string, command string, args ...string) {
	t.Helper()
	if out, err := execCommand(dir, command, args...); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", command, args, err, out)
	}
}

func execCommand(dir string, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeScreenshotFixture creates a file that exceeds DefaultMinScreenshotSize.
// The content parameter is used as a prefix to ensure uniqueness across scenarios,
// with padding added to reach the minimum size.
func writeScreenshotFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	minSize := int(DefaultMinScreenshotSize) + 1
	// Prepend PNG magic bytes so the format check passes.
	padded := "\x89PNG\r\n\x1a\n" + content
	for len(padded) < minSize {
		padded += "\x00"
	}
	return writeFixture(t, dir, name, padded)
}

// filePreNotesForAll files a pre-test note for every scenario declared on
// the run, using the same session id for each one. Tests need this because
// record calls now refuse evidence without a pre-note; the default test
// fixture keeps each test focused on the behaviour under test rather than
// the pre-note gate itself. When a test uses multiple sessions per scenario
// it should file additional pre-notes with FilePreNote directly.
func filePreNotesForAll(t *testing.T, store *Store, run Run, session, notes string) {
	t.Helper()
	if len(notes) < MinVerificationLength {
		t.Fatalf("filePreNotesForAll notes too short (%d chars); test fixtures must supply real pre-notes", len(notes))
	}
	for _, scenario := range run.Scenarios {
		if _, err := FilePreNote(store, run, scenario.ID, session, notes); err != nil {
			t.Fatalf("file pre-note for scenario %s: %v", scenario.ID, err)
		}
	}
}

// filePreNote files a single pre-test note for the given scenario+session
// pair with a canned note body. It is the minimal helper used by tests that
// only need to satisfy the record gate for one scenario.
func filePreNote(t *testing.T, store *Store, run Run, scenario, session string) {
	t.Helper()
	if _, err := FilePreNote(store, run, scenario, session, testPreNoteText); err != nil {
		t.Fatalf("file pre-note for scenario %s session %s: %v", scenario, session, err)
	}
}

// testPreNoteText is a fixed pre-test note body that safely clears the
// 20-character minimum shared with proctor verify notes.
const testPreNoteText = "about to verify this scenario end to end with the described fixture screenshots"

// verifyAllScenarios walks every evidence entry in the ledger and calls
// VerifyEvidence once per (scenario, session) pair using a fixed observation
// note. Most tests only need every recorded screenshot to be flipped to
// complete so the scenario can be evaluated; this helper keeps that
// boilerplate out of the individual test bodies.
func verifyAllScenarios(t *testing.T, store *Store, run Run, notes string) {
	t.Helper()
	if len(notes) < MinVerificationLength {
		t.Fatalf("verifyAllScenarios notes too short (%d chars); test fixtures must supply real observations", len(notes))
	}
	records, err := store.loadEvidenceRaw(run)
	if err != nil {
		t.Fatalf("load evidence for verification: %v", err)
	}
	seen := map[string]bool{}
	for _, item := range records {
		if item.Surface == SurfaceCurl {
			continue
		}
		key := item.ScenarioID + "\x00" + item.Provenance.SessionID
		if seen[key] {
			continue
		}
		seen[key] = true
		if item.Status == EvidenceStatusComplete {
			continue
		}
		if err := VerifyEvidence(store, run, item.ScenarioID, item.Provenance.SessionID, notes); err != nil {
			t.Fatalf("verify evidence for scenario %s session %s: %v", item.ScenarioID, item.Provenance.SessionID, err)
		}
	}
}

// testObservationNotes is a fixed verification that safely clears the 40-character
// minimum and judgment-word requirement for proctor verify. Tests use this
// when they only need the verification gate to pass and the exact text is
// not under test.
const testObservationNotes = "This satisfies the contract because the screenshot shows the expected view with the described UI elements visible"

// logStepsForAll files a log entry for every scenario in the run. Tests
// need this because the done gate now requires at least one log entry per
// scenario. The default fixture keeps each test focused on the behaviour
// under test rather than the log gate itself.
func logStepsForAll(t *testing.T, store *Store, run Run, repo, session string) {
	t.Helper()
	for _, scenario := range run.Scenarios {
		shot := writeScreenshotFixture(t, repo, "log-"+scenario.ID+".png", "log-image-"+scenario.ID)
		if _, err := LogStep(store, run, LogStepOptions{
			ScenarioID:     scenario.ID,
			SessionID:      session,
			Surface:        surfaceForPlatform(run.Platform),
			ScreenshotPath: shot,
			Action:         "verified the " + scenario.ID + " scenario end to end with fixture",
			Observation:    "screenshot shows the expected state for " + scenario.ID + " with correct UI elements",
			Comparison:     "what is visible matches the " + scenario.ID + " scenario requirements as described",
		}); err != nil {
			t.Fatalf("log step for scenario %s: %v", scenario.ID, err)
		}
	}
}

func surfaceForPlatform(platform string) string {
	switch normalizePlatform(platform) {
	case PlatformIOS:
		return SurfaceIOS
	case PlatformCLI:
		return SurfaceCLI
	case PlatformDesktop:
		return SurfaceDesktop
	default:
		return SurfaceBrowser
	}
}

func containsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

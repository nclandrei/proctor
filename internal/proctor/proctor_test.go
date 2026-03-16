package proctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
	if !strings.Contains(reportText, "Proctor Report") || !strings.Contains(reportText, "summary-grid") {
		t.Fatalf("expected report html to include the new summary layout, got:\n%s", reportText)
	}
	if !strings.Contains(reportText, "Scenario Rollup") || !strings.Contains(reportText, "--bg: #081017") {
		t.Fatalf("expected report html to include the dark rollup layout, got:\n%s", reportText)
	}
	if !strings.Contains(reportText, `<meta name="color-scheme" content="dark">`) || !strings.Contains(reportText, "color-scheme: dark;") {
		t.Fatalf("expected report html to force dark mode, got:\n%s", reportText)
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
	if err == nil || !strings.Contains(err.Error(), "--curl must be either required or skip") {
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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")

	if err := RecordBrowser(store, run, BrowserRecordOptions{
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
	}); err != nil {
		t.Fatal(err)
	}

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

	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")
	successReport := writeFixture(t, repo, "success-report.json", sampleBrowserReport("http://127.0.0.1:3000/dashboard", 0, 0, 0, 0))
	failureReport := writeFixture(t, repo, "failure-report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 0, 0, 0, 1))

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: successReport,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
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
			"desktop-failure": desktopShot,
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
			"desktop-edge": desktopShot,
		}
		if scenario.Category == "mobile or responsive behavior" {
			screenshots["mobile-edge"] = mobileShot
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
		Command:    []string{"/bin/sh", "-lc", "printf 'HTTP/1.1 200 OK\\nContent-Type: application/json\\n\\n{\"ok\":true}'"},
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
		Command:    []string{"/bin/sh", "-lc", "printf 'HTTP/1.1 401 Unauthorized\\nContent-Type: application/json\\n\\n{\"error\":\"invalid\"}'"},
		PassAssertions: []string{
			"status = 401",
			"body contains invalid",
		},
	}); err != nil {
		t.Fatal(err)
	}

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")

	for _, scenarioID := range []string{"happy-path", "failure-path"} {
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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")

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

	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID: "happy-path",
		Command:    []string{"/bin/sh", "-lc", "printf 'HTTP/1.1 500 Internal Server Error\\nContent-Type: application/json\\n\\n{\"error\":\"boom\"}'"},
		PassAssertions: []string{
			"status = 200",
		},
	}); err != nil {
		t.Fatal(err)
	}

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")

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
	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")

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

func containsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

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
		if err := RecordBrowser(store, run, BrowserRecordOptions{
			ScenarioID: scenario.ID,
			SessionID:  "browser-1",
			ReportPath: successReport,
			Screenshots: map[string]string{
				"desktop-edge": desktopShot,
			},
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

func sampleBrowserReport(finalURL string, consoleErrors, pageErrors, failedRequests, httpErrors int) string {
	return `{
  "desktop": {
    "title": "Login",
    "finalUrl": "` + finalURL + `",
    "issues": {
      "consoleErrors": ` + itoa(consoleErrors) + `,
      "consoleWarnings": 0,
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

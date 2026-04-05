package proctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateDesktopRunWritesExpectedMetadata(t *testing.T) {
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

	if run.Platform != PlatformDesktop {
		t.Fatalf("expected desktop platform, got %q", run.Platform)
	}
	if run.Desktop.Name != "Firefox" {
		t.Fatalf("expected desktop app name metadata, got %#v", run.Desktop)
	}
	if !run.HappyPath.DesktopRequired || run.HappyPath.BrowserRequired {
		t.Fatalf("expected happy path to require desktop evidence only, got %#v", run.HappyPath)
	}

	contract, err := os.ReadFile(filepath.Join(store.RunDir(run), "contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	contractText := string(contract)
	for _, needle := range []string{
		"- Platform: `desktop`",
		"- Desktop app: `Firefox`",
		"window management, resize, and multi-monitor",
		"drag-drop, clipboard, and system integration",
	} {
		if !strings.Contains(contractText, needle) {
			t.Fatalf("expected desktop contract to mention %q, got:\n%s", needle, contractText)
		}
	}
	if strings.Contains(contractText, "mobile or responsive behavior") {
		t.Fatalf("expected desktop contract to use desktop edge-case categories, got:\n%s", contractText)
	}
}

func TestCreateDesktopRunRequiresAppName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	_, err = CreateRun(store, repo, StartOptions{
		Platform:       PlatformDesktop,
		Feature:        "missing app name",
		CurlMode:       "skip",
		CurlSkipReason: "UI-only test",
		HappyPath:      "happy",
		FailurePath:    "failure",
		EdgeCaseInputs: allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformDesktop)),
	})
	if err == nil || !strings.Contains(err.Error(), "--app-name is required when --platform desktop") {
		t.Fatalf("expected app name validation, got %v", err)
	}
}

func TestRecordDesktopEvaluatesStructuredAssertions(t *testing.T) {
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

	filePreNote(t, store, run, "happy-path", "firefox-desktop-1")
	if err := RecordDesktop(store, run, DesktopRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "firefox-desktop-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"window": screenshot,
		},
		PassAssertions: []string{
			"app_name contains Firefox",
			"crashes = 0",
			"screenshot = true",
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
	if evidence[0].Desktop == nil {
		t.Fatalf("expected desktop evidence payload, got %#v", evidence[0])
	}
	for _, assertion := range evidence[0].Assertions {
		if assertion.Result != AssertionPass {
			t.Fatalf("expected passing assertion, got %#v", assertion)
		}
	}
}

func TestDesktopImplicitHealthChecksFailWhenUnaccounted(t *testing.T) {
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

	report := writeFixture(t, repo, "desktop-report.json", sampleDesktopReport("Firefox", "org.mozilla.firefox", "running", "Bookmark Manager", 1, 0))
	screenshot := writeScreenshotFixture(t, repo, "window.png", "window-image")

	filePreNote(t, store, run, "happy-path", "firefox-desktop-1")
	// Record with implicit health checks that will fail; error expected.
	_ = RecordDesktop(store, run, DesktopRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "firefox-desktop-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"window": screenshot,
		},
		PassAssertions: []string{
			"app_name contains Firefox",
		},
	})

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	foundImplicitFailure := false
	for _, assertion := range evidence[0].Assertions {
		if assertion.Description == "crashes = 0" && assertion.Result == AssertionFail {
			foundImplicitFailure = true
		}
	}
	if !foundImplicitFailure {
		t.Fatalf("expected implicit crash assertion to fail")
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].DesktopOK {
		t.Fatalf("expected desktop evidence to fail implicit checks")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].DesktopIssues, "assertion failed: crashes = 0") {
		t.Fatalf("expected crash assertion failure, got %#v", eval.ScenarioEvaluations[0].DesktopIssues)
	}
}

func TestDesktopFailAssertionReportsUnexpectedPassClearly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, desktopMinimalStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "desktop-report.json", sampleDesktopReport("Firefox", "org.mozilla.firefox", "running", "Bookmark Manager", 0, 0))
	screenshot := writeScreenshotFixture(t, repo, "window.png", "window-image")

	filePreNote(t, store, run, "happy-path", "firefox-desktop-1")
	// Record with a fail-assert that unexpectedly passes; error expected.
	_ = RecordDesktop(store, run, DesktopRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "firefox-desktop-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"window": screenshot,
		},
		PassAssertions: []string{
			"crashes = 0",
		},
		FailAssertions: []string{
			"app_name contains Firefox",
		},
	})

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].DesktopOK {
		t.Fatalf("expected desktop evidence to fail when an inverted assertion passes")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].DesktopIssues, "assertion failed: NOT (app_name contains Firefox)") {
		t.Fatalf("expected inverted assertion description, got %#v", eval.ScenarioEvaluations[0].DesktopIssues)
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].DesktopIssues, "expected this assertion to fail, but it passed") {
		t.Fatalf("expected inverted assertion explanation, got %#v", eval.ScenarioEvaluations[0].DesktopIssues)
	}
}

func TestDonePassesWhenRequiredDesktopEvidenceExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, desktopMinimalStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "desktop-report.json", sampleDesktopReport("Firefox", "org.mozilla.firefox", "running", "Bookmark Manager", 0, 0))

	for i, scenarioID := range []string{"happy-path", "failure-path"} {
		screenshot := writeScreenshotFixture(t, repo, fmt.Sprintf("window-%d.png", i), fmt.Sprintf("window-image-%s", scenarioID))
		filePreNote(t, store, run, scenarioID, "firefox-desktop-1")
		if err := RecordDesktop(store, run, DesktopRecordOptions{
			ScenarioID: scenarioID,
			SessionID:  "firefox-desktop-1",
			ReportPath: report,
			Screenshots: map[string]string{
				"window": screenshot,
			},
			PassAssertions: []string{
				"app_name contains Firefox",
				"crashes = 0",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	verifyAllScenarios(t, store, run, testObservationNotes)

	eval, err := CompleteRun(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if !eval.Complete {
		t.Fatalf("expected desktop run to pass, got %#v", eval)
	}

	contract, err := os.ReadFile(filepath.Join(store.RunDir(run), "contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"App: `Firefox`",
		"Session: `firefox-desktop-1`",
	} {
		if !strings.Contains(string(contract), needle) {
			t.Fatalf("expected desktop contract to mention %q, got:\n%s", needle, contract)
		}
	}
}

func sampleDesktopStartOptions() StartOptions {
	return StartOptions{
		Platform:        PlatformDesktop,
		Feature:         "bookmark manager",
		DesktopAppName:  "Firefox",
		DesktopBundleID: "org.mozilla.firefox",
		CurlMode:        "skip",
		CurlSkipReason:  "UI-only desktop verification",
		HappyPath:       "bookmark manager opens and lists saved bookmarks",
		FailurePath:     "empty bookmark list shows a helpful prompt",
		EdgeCaseInputs: []string{
			"validation and malformed input=N/A: no freeform input in this flow",
			"empty or missing input=N/A: no required input in this flow",
			"retry or double-submit=N/A: no repeated mutation in this flow",
			"loading, latency, and race conditions=N/A: instant local operation",
			"network or server failure=N/A: no backend dependency in this flow",
			"auth and session state=N/A: no authentication required",
			"window management, resize, and multi-monitor=bookmark manager resizes cleanly",
			"drag-drop, clipboard, and system integration=N/A: no drag-drop in this flow",
			"keyboard shortcuts and accessibility=N/A: this pass is visual only",
			"any feature-specific risks=N/A: no additional risks",
		},
	}
}

func desktopMinimalStartOptions() StartOptions {
	return StartOptions{
		Platform:        PlatformDesktop,
		Feature:         "bookmark manager",
		DesktopAppName:  "Firefox",
		DesktopBundleID: "org.mozilla.firefox",
		CurlMode:        "skip",
		CurlSkipReason:  "UI-only desktop verification",
		HappyPath:       "bookmark manager opens and lists saved bookmarks",
		FailurePath:     "empty bookmark list shows a helpful prompt",
		EdgeCaseInputs:  allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformDesktop)),
	}
}

func sampleDesktopReport(appName, bundleID, state, windowTitle string, crashes, fatalLogs int) string {
	return `{
  "app": {
    "name": "` + appName + `",
    "bundleId": "` + bundleID + `",
    "state": "` + state + `",
    "windowTitle": "` + windowTitle + `"
  },
  "issues": {
    "crashes": ` + itoa(crashes) + `,
    "fatalLogs": ` + itoa(fatalLogs) + `
  }
}`
}

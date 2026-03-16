package proctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateIOSRunWritesExpectedTargetMetadata(t *testing.T) {
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

	if run.Platform != PlatformIOS {
		t.Fatalf("expected ios platform, got %q", run.Platform)
	}
	if run.IOS.Scheme != "Pagena" {
		t.Fatalf("expected ios scheme metadata, got %#v", run.IOS)
	}
	if !run.HappyPath.IOSRequired || run.HappyPath.BrowserRequired {
		t.Fatalf("expected happy path to require ios evidence only, got %#v", run.HappyPath)
	}

	contract, err := os.ReadFile(filepath.Join(store.RunDir(run), "contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	contractText := string(contract)
	for _, needle := range []string{
		"- Platform: `ios`",
		"- iOS scheme: `Pagena`",
		"- iOS bundle ID: `com.example.pagena`",
		"- Simulator: `iPhone 16 Pro`",
		"device traits, orientation, and layout",
		"app lifecycle, relaunch, and state persistence",
	} {
		if !strings.Contains(contractText, needle) {
			t.Fatalf("expected ios contract to mention %q, got:\n%s", needle, contractText)
		}
	}
	if strings.Contains(contractText, "mobile or responsive behavior") {
		t.Fatalf("expected ios contract to use ios edge-case categories, got:\n%s", contractText)
	}
}

func TestCreateIOSRunRequiresSchemeAndBundleID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	_, err = CreateRun(store, repo, StartOptions{
		Platform:       PlatformIOS,
		Feature:        "missing scheme",
		IOSBundleID:    "com.example.pagena",
		CurlMode:       "skip",
		CurlSkipReason: "UI-only test",
		HappyPath:      "happy",
		FailurePath:    "failure",
		EdgeCaseInputs: allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformIOS)),
	})
	if err == nil || !strings.Contains(err.Error(), "--ios-scheme is required when --platform ios") {
		t.Fatalf("expected ios scheme validation, got %v", err)
	}

	_, err = CreateRun(store, repo, StartOptions{
		Platform:       PlatformIOS,
		Feature:        "missing bundle id",
		IOSScheme:      "Pagena",
		CurlMode:       "skip",
		CurlSkipReason: "UI-only test",
		HappyPath:      "happy",
		FailurePath:    "failure",
		EdgeCaseInputs: allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformIOS)),
	})
	if err == nil || !strings.Contains(err.Error(), "--ios-bundle-id is required when --platform ios") {
		t.Fatalf("expected ios bundle id validation, got %v", err)
	}
}

func TestRecordIOSEvaluatesStructuredAssertions(t *testing.T) {
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
	screenshot := writeFixture(t, repo, "library.png", "library-screen")

	if err := RecordIOS(store, run, IOSRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "pagena-library-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"library-screen": screenshot,
		},
		PassAssertions: []string{
			"screen contains Library",
			"bundle_id = com.example.pagena",
			"simulator contains iPhone 16 Pro",
			"app_launch = true",
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
	if evidence[0].IOS == nil {
		t.Fatalf("expected ios evidence payload, got %#v", evidence[0])
	}
	for _, assertion := range evidence[0].Assertions {
		if assertion.Result != AssertionPass {
			t.Fatalf("expected passing assertion, got %#v", assertion)
		}
	}
}

func TestIOSImplicitHealthChecksFailWhenIssuesAreUnaccountedFor(t *testing.T) {
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

	report := writeFixture(t, repo, "ios-report.json", sampleIOSReport("com.example.pagena", "Library", "foreground", "iPhone 16 Pro", "iOS 18.2", 0, 1, 0))
	screenshot := writeFixture(t, repo, "library.png", "library-screen")

	if err := RecordIOS(store, run, IOSRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "pagena-library-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"library-screen": screenshot,
		},
		PassAssertions: []string{
			"screen contains Library",
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
	if eval.ScenarioEvaluations[0].IOSOK {
		t.Fatalf("expected ios evidence to fail implicit checks")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].IOSIssues, "assertion failed: crashes = 0") {
		t.Fatalf("expected crash assertion failure, got %#v", eval.ScenarioEvaluations[0].IOSIssues)
	}
}

func TestIOSFailAssertionReportsUnexpectedPassClearly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, iosMinimalStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "ios-report.json", sampleIOSReport("com.example.pagena", "Library", "foreground", "iPhone 16 Pro", "iOS 18.2", 0, 0, 0))
	screenshot := writeFixture(t, repo, "library.png", "library-screen")

	if err := RecordIOS(store, run, IOSRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "pagena-library-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"library-screen": screenshot,
		},
		PassAssertions: []string{
			"bundle_id = com.example.pagena",
		},
		FailAssertions: []string{
			"screen contains Library",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].IOSOK {
		t.Fatalf("expected ios evidence to fail when an inverted assertion passes")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].IOSIssues, "assertion failed: NOT (screen contains Library)") {
		t.Fatalf("expected inverted assertion description, got %#v", eval.ScenarioEvaluations[0].IOSIssues)
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].IOSIssues, "expected this assertion to fail, but it passed") {
		t.Fatalf("expected inverted assertion explanation, got %#v", eval.ScenarioEvaluations[0].IOSIssues)
	}
}

func TestDonePassesWhenRequiredIOSEvidenceExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, iosMinimalStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "ios-report.json", sampleIOSReport("com.example.pagena", "Library", "foreground", "iPhone 16 Pro", "iOS 18.2", 0, 0, 0))
	screenshot := writeFixture(t, repo, "library.png", "library-screen")

	for _, scenarioID := range []string{"happy-path", "failure-path"} {
		if err := RecordIOS(store, run, IOSRecordOptions{
			ScenarioID: scenarioID,
			SessionID:  "pagena-library-1",
			ReportPath: report,
			Screenshots: map[string]string{
				"library-screen": screenshot,
			},
			PassAssertions: []string{
				"screen contains Library",
				"bundle_id = com.example.pagena",
				"app_launch = true",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	eval, err := CompleteRun(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if !eval.Complete {
		t.Fatalf("expected ios run to pass, got %#v", eval)
	}

	contract, err := os.ReadFile(filepath.Join(store.RunDir(run), "contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"Bundle ID: `com.example.pagena`",
		"Screen: `Library`",
		"Simulator: `iPhone 16 Pro`",
		"Session: `pagena-library-1`",
	} {
		if !strings.Contains(string(contract), needle) {
			t.Fatalf("expected ios contract to mention %q, got:\n%s", needle, contract)
		}
	}
}

func TestIOSReportRequiresBundleIDAndScreen(t *testing.T) {
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

	report := writeFixture(t, repo, "ios-report.json", `{
  "simulator": {
    "name": "iPhone 16 Pro",
    "runtime": "iOS 18.2"
  },
  "app": {
    "bundleId": "",
    "screen": "",
    "state": "foreground"
  },
  "issues": {
    "launchErrors": 0,
    "crashes": 0,
    "fatalLogs": 0
  }
}`)
	screenshot := writeFixture(t, repo, "library.png", "library-screen")

	if err := RecordIOS(store, run, IOSRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "pagena-library-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"library-screen": screenshot,
		},
		PassAssertions: []string{
			"simulator contains iPhone 16 Pro",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].IOSOK {
		t.Fatalf("expected ios evidence to fail without required report metadata")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].IOSIssues, "ios report is missing an app bundle id") {
		t.Fatalf("expected missing bundle id issue, got %#v", eval.ScenarioEvaluations[0].IOSIssues)
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].IOSIssues, "ios report is missing a screen description") {
		t.Fatalf("expected missing screen issue, got %#v", eval.ScenarioEvaluations[0].IOSIssues)
	}
}

func sampleIOSStartOptions() StartOptions {
	return StartOptions{
		Platform:       PlatformIOS,
		Feature:        "library flow",
		IOSScheme:      "Pagena",
		IOSBundleID:    "com.example.pagena",
		IOSSimulator:   "iPhone 16 Pro",
		CurlMode:       "skip",
		CurlSkipReason: "UI-only verification",
		HappyPath:      "launching the app lands on the library screen",
		FailurePath:    "an unavailable chapter shows a visible fallback state",
		EdgeCaseInputs: []string{
			"validation and malformed input=N/A: no freeform input in this flow",
			"empty or missing input=N/A: no required input in this flow",
			"retry or double-submit=N/A: no repeated mutation in this flow",
			"loading, latency, and race conditions=loading placeholder settles into the library without duplicate cards",
			"network or server failure=offline launch shows a recoverable empty state",
			"auth and session state=N/A: anonymous browsing only",
			"app lifecycle, relaunch, and state persistence=foregrounding after background keeps the same library selection",
			"device traits, orientation, and layout=library remains readable in portrait and compact width",
			"accessibility, dynamic type, and keyboard behavior=N/A: this pass is visual only",
			"any feature-specific risks=N/A: no additional feature-specific risks",
		},
	}
}

func iosMinimalStartOptions() StartOptions {
	return StartOptions{
		Platform:       PlatformIOS,
		Feature:        "library flow",
		IOSScheme:      "Pagena",
		IOSBundleID:    "com.example.pagena",
		IOSSimulator:   "iPhone 16 Pro",
		CurlMode:       "skip",
		CurlSkipReason: "UI-only verification",
		HappyPath:      "launching the app lands on the library screen",
		FailurePath:    "an unavailable chapter shows a visible fallback state",
		EdgeCaseInputs: allNAEdgeCases(EdgeCaseCategoriesForPlatform(PlatformIOS)),
	}
}

func sampleIOSReport(bundleID, screen, state, simulatorName, runtime string, launchErrors, crashes, fatalLogs int) string {
	return `{
  "simulator": {
    "name": "` + simulatorName + `",
    "runtime": "` + runtime + `"
  },
  "app": {
    "bundleId": "` + bundleID + `",
    "screen": "` + screen + `",
    "state": "` + state + `"
  },
  "issues": {
    "launchErrors": ` + itoa(launchErrors) + `,
    "crashes": ` + itoa(crashes) + `,
    "fatalLogs": ` + itoa(fatalLogs) + `
  }
}`
}

func allNAEdgeCases(categories []string) []string {
	inputs := make([]string, 0, len(categories))
	for _, category := range categories {
		inputs = append(inputs, category+"=N/A: covered by this test")
	}
	return inputs
}

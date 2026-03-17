package proctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateRunCLIRequiresCLICommandAndRejectsWebOnlyFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	_, err = CreateRun(store, repo, StartOptions{
		Platform:    PlatformCLI,
		Feature:     "cli run without command",
		HappyPath:   "happy",
		FailurePath: "failure",
	})
	if err == nil || !strings.Contains(err.Error(), "--cli-command is required when --platform cli") {
		t.Fatalf("expected missing cli command validation, got %v", err)
	}

	_, err = CreateRun(store, repo, StartOptions{
		Platform:       PlatformCLI,
		Feature:        "cli run with url",
		CLICommand:     "demo help",
		BrowserURL:     "http://127.0.0.1:3000",
		HappyPath:      "happy",
		FailurePath:    "failure",
		EdgeCaseInputs: cliNAEdgeCases(),
	})
	if err == nil || !strings.Contains(err.Error(), "--url and --ios-* flags are only valid on their matching platforms") {
		t.Fatalf("expected cli/web flag mismatch validation, got %v", err)
	}
}

func TestRecordCLICompletesRunWithStructuredAssertions(t *testing.T) {
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

	terminalShot := writeFixture(t, repo, "terminal.png", "terminal-image")
	happyTranscript := writeFixture(t, repo, "happy-pane.txt", "Usage:\n  demo help\nonboarding prompt")
	failureTranscript := writeFixture(t, repo, "failure-pane.txt", "error: prompt not found")
	happyExitCode := 0
	failureExitCode := 2

	if err := RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-session-1",
		Command:        "demo help",
		TranscriptPath: happyTranscript,
		ExitCode:       &happyExitCode,
		Screenshots: map[string]string{
			"terminal-happy": terminalShot,
		},
		PassAssertions: []string{
			"output contains Usage:",
			"command contains demo",
			"session contains cli-session",
			"tool = terminal-session",
			"exit_code = 0",
			"screenshot = true",
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "failure-path",
		SessionID:      "cli-session-1",
		Command:        "demo help missing",
		TranscriptPath: failureTranscript,
		ExitCode:       &failureExitCode,
		Screenshots: map[string]string{
			"terminal-failure": terminalShot,
		},
		PassAssertions: []string{
			"output contains prompt not found",
			"exit_code = 2",
			"screenshot = true",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := CompleteRun(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if !eval.Complete {
		t.Fatalf("expected CLI run to pass, got %#v", eval)
	}
	if len(eval.GlobalMissing) != 0 {
		t.Fatalf("expected CLI run to have no browser-global gaps, got %#v", eval.GlobalMissing)
	}

	contract, err := os.ReadFile(filepath.Join(store.RunDir(run), "contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	contractText := string(contract)
	for _, needle := range []string{
		"Verification surface: `CLI`",
		"CLI command: `demo help`",
		"Session: `cli-session-1`",
		"Command: `demo help`",
		"Transcript preview: `Usage:",
	} {
		if !strings.Contains(contractText, needle) {
			t.Fatalf("expected contract to include %q, got:\n%s", needle, contractText)
		}
	}
}

func TestRecordCLIRequiresTranscriptAndScreenshot(t *testing.T) {
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

	err = RecordCLI(store, run, CLIRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "cli-session-1",
		Command:    "demo help",
		Screenshots: map[string]string{
			"terminal": writeFixture(t, repo, "terminal.png", "terminal-image"),
		},
		PassAssertions: []string{"screenshot = true"},
	})
	if err == nil || !strings.Contains(err.Error(), "cli evidence requires --transcript") {
		t.Fatalf("expected missing transcript validation, got %v", err)
	}

	err = RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-session-1",
		Command:        "demo help",
		TranscriptPath: writeFixture(t, repo, "pane.txt", "Usage:\n  demo help"),
		PassAssertions: []string{"output contains Usage"},
	})
	if err == nil || !strings.Contains(err.Error(), "cli evidence requires at least one --screenshot") {
		t.Fatalf("expected missing screenshot validation, got %v", err)
	}
}

func TestCLIAssertionFailureBlocksScenario(t *testing.T) {
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

	exitCode := 0
	if err := RecordCLI(store, run, CLIRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "cli-session-1",
		Command:        "demo help",
		TranscriptPath: writeFixture(t, repo, "pane.txt", "Usage:\n  demo help"),
		ExitCode:       &exitCode,
		Screenshots: map[string]string{
			"terminal": writeFixture(t, repo, "terminal.png", "terminal-image"),
		},
		PassAssertions: []string{
			"output contains definitely-missing-text",
		},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := Evaluate(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if eval.ScenarioEvaluations[0].CLIOK {
		t.Fatalf("expected CLI evidence to fail")
	}
	if !containsSubstring(eval.ScenarioEvaluations[0].CLIIssues, "assertion failed: output contains definitely-missing-text") {
		t.Fatalf("expected CLI issues to include the failed assertion, got %#v", eval.ScenarioEvaluations[0].CLIIssues)
	}
}

func TestCLIFailAssertUsesFinalizeSemanticsLikeOtherSurfaces(t *testing.T) {
	exitCode := 0
	data := CLIData{
		Command:   "demo help",
		SessionID: "cli-session-1",
		Tool:      "terminal-session",
		ExitCode:  &exitCode,
	}
	transcript := "Usage:\n  demo help"
	artifacts := []Artifact{{Kind: ArtifactImage, Label: "terminal"}}

	// A fail-assert where the underlying expression DOES match (so the
	// negation should mark it as failed with a diagnostic message).
	assertions, err := EvaluateCLIAssertions(
		nil,
		[]string{"output contains Usage"},
		data, transcript, artifacts,
	)
	if err != nil {
		t.Fatal(err)
	}

	var failAssert *Assertion
	for idx := range assertions {
		if strings.Contains(assertions[idx].Description, "Usage") {
			failAssert = &assertions[idx]
			break
		}
	}
	if failAssert == nil {
		t.Fatal("expected to find the fail-assert in the output")
	}

	// Must have NOT (...) prefix like browser, iOS, and curl surfaces.
	if !strings.HasPrefix(failAssert.Description, "NOT (") {
		t.Fatalf("expected Description to have NOT (...) prefix, got %q", failAssert.Description)
	}

	// Must be marked as failed (the underlying expression matched, so the
	// negation means it failed).
	if failAssert.Result != AssertionFail {
		t.Fatalf("expected Result=fail for a negated assertion that matched, got %q", failAssert.Result)
	}

	// Must carry the diagnostic message like other surfaces.
	if failAssert.Message == "" {
		t.Fatal("expected a non-empty Message explaining why the negated assertion failed")
	}
	if !strings.Contains(failAssert.Message, "expected this assertion to fail") {
		t.Fatalf("expected diagnostic message, got %q", failAssert.Message)
	}

	// Also test the happy path: fail-assert where expression does NOT match
	// should pass with no message.
	assertions2, err := EvaluateCLIAssertions(
		nil,
		[]string{"output contains definitely-missing"},
		data, transcript, artifacts,
	)
	if err != nil {
		t.Fatal(err)
	}

	var passAssert *Assertion
	for idx := range assertions2 {
		if strings.Contains(assertions2[idx].Description, "definitely-missing") {
			passAssert = &assertions2[idx]
			break
		}
	}
	if passAssert == nil {
		t.Fatal("expected to find the fail-assert in the output")
	}

	if !strings.HasPrefix(passAssert.Description, "NOT (") {
		t.Fatalf("expected Description to have NOT (...) prefix, got %q", passAssert.Description)
	}
	if passAssert.Result != AssertionPass {
		t.Fatalf("expected Result=pass for a negated assertion that did not match, got %q", passAssert.Result)
	}
}

func sampleCLIStartOptions() StartOptions {
	return StartOptions{
		Platform:       PlatformCLI,
		Feature:        "cli prompt inspection",
		CLICommand:     "demo help",
		HappyPath:      "help output is readable",
		FailurePath:    "unknown argument fails clearly",
		EdgeCaseInputs: cliNAEdgeCases(),
	}
}

func cliNAEdgeCases() []string {
	inputs := make([]string, 0, len(EdgeCaseCategoriesForPlatform(PlatformCLI)))
	for _, category := range EdgeCaseCategoriesForPlatform(PlatformCLI) {
		inputs = append(inputs, category+"=N/A: covered by this test")
	}
	return inputs
}

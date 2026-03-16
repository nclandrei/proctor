package main

import (
	"strings"
	"testing"
)

func TestCommandHelpSupportsNestedSubcommandsWithoutActiveRun(t *testing.T) {
	text, ok, err := commandHelp([]string{"record", "browser", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "proctor record browser") {
		t.Fatalf("expected browser help text, got:\n%s", text)
	}
	if !strings.Contains(text, "--report /abs/path/report.json") {
		t.Fatalf("expected browser help to include report flag example, got:\n%s", text)
	}
	if !strings.Contains(text, "console_warnings = 0") {
		t.Fatalf("expected browser help to include console warning assertions, got:\n%s", text)
	}
}

func TestHelpCommandSupportsNestedTopics(t *testing.T) {
	text, ok, err := commandHelp([]string{"help", "record", "curl"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "proctor record curl") {
		t.Fatalf("expected curl help text, got:\n%s", text)
	}
	if !strings.Contains(text, "-- <command>") {
		t.Fatalf("expected curl help to describe wrapped command syntax, got:\n%s", text)
	}
}

func TestRootHelpMentionsAgentAgnosticWorkflow(t *testing.T) {
	text, ok, err := commandHelp([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"Codex, Claude Code",
		"proctor start --help",
		"proctor record browser --help",
		"--curl scenario",
		"report.html is always rendered in dark mode",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestRootHelpMentionsMandatoryMobileCoverage(t *testing.T) {
	text, ok, err := commandHelp([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "mobile proof is mandatory") {
		t.Fatalf("expected root help to describe mandatory mobile coverage, got:\n%s", text)
	}
}

func TestRecordBrowserHelpMentionsRunWideMobileRequirement(t *testing.T) {
	text, ok, err := commandHelp([]string{"record", "browser", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "every web run must record at least one desktop screenshot and at least one mobile screenshot") {
		t.Fatalf("expected record browser help to describe run-wide mobile coverage, got:\n%s", text)
	}
	if !strings.Contains(text, "console warnings are recorded in the report but stay non-blocking unless you assert them explicitly") {
		t.Fatalf("expected record browser help to explain the warning policy, got:\n%s", text)
	}
}

func TestStartHelpExplainsScenarioLevelCurlPlanning(t *testing.T) {
	text, ok, err := commandHelp([]string{"start", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"--curl required|scenario|skip",
		`--curl-endpoint "happy-path=POST /api/login"`,
		"require curl only for named risky scenarios",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected start help to mention %q, got:\n%s", needle, text)
		}
	}
}

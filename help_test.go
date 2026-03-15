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
		"report.html is always rendered in dark mode",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected help to mention %q, got:\n%s", needle, text)
		}
	}
}

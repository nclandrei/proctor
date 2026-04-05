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

func TestCommandHelpSupportsCLINestedSubcommandsWithoutActiveRun(t *testing.T) {
	text, ok, err := commandHelp([]string{"record", "cli", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "proctor record cli") {
		t.Fatalf("expected cli help text, got:\n%s", text)
	}
	if !strings.Contains(text, "--transcript /abs/path/pane.txt") {
		t.Fatalf("expected cli help to include transcript example, got:\n%s", text)
	}
	if !strings.Contains(text, "tmux or an equivalent persistent multiplexer") {
		t.Fatalf("expected cli help to include terminal guidance, got:\n%s", text)
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
	if !strings.Contains(text, "match one of that scenario's declared curl endpoints") {
		t.Fatalf("expected curl help to mention endpoint contract enforcement, got:\n%s", text)
	}
}

func TestCommandHelpSupportsIOSNestedSubcommandsWithoutActiveRun(t *testing.T) {
	text, ok, err := commandHelp([]string{"record", "ios", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "proctor record ios") {
		t.Fatalf("expected ios help text, got:\n%s", text)
	}
	if !strings.Contains(text, "--report /abs/path/ios-report.json") {
		t.Fatalf("expected ios help to include report flag example, got:\n%s", text)
	}
	if !strings.Contains(text, "app_launch = true") {
		t.Fatalf("expected ios help to include launch assertions, got:\n%s", text)
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
		"Reading this help text is not the task",
		"Mandatory next step for the agent",
		"proctor start --help",
		"proctor record browser --help",
		"proctor record cli --help",
		"--curl scenario",
		"proctor record ios --help",
		"report.html is a plain document with a light theme",
		"known-good local capture workflows based on tools found on PATH",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestRootHelpMentionsCLIWorkflow(t *testing.T) {
	text, ok, err := commandHelp([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"Typical CLI workflow",
		"--platform cli",
		"--cli-command \"magellan prompts inspect onboarding\"",
		"tmux or an equivalent",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected root help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestRootHelpMentionsIOSWorkflow(t *testing.T) {
	text, ok, err := commandHelp([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"Typical iOS workflow",
		"--platform ios",
		"--ios-scheme Pagena",
		"use your own simulator tooling",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected root help to mention %q, got:\n%s", needle, text)
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
	withStubbedLookPath(t, "agent-browser", "curl")
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
	if !strings.Contains(text, "`agent-browser` detected on PATH") {
		t.Fatalf("expected record browser help to include local browser recommendations, got:\n%s", text)
	}
}

func TestStartHelpMentionsDiffDrivenFeatureSelection(t *testing.T) {
	text, ok, err := commandHelp([]string{"start", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"inspect the current repo diff",
		"user-visible change that actually needs verification",
		"generic smoke test that is unrelated to the current diff",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected start help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestRecordIOSHelpMentionsImplicitHealthChecks(t *testing.T) {
	withStubbedLookPath(t, "xcrun")
	text, ok, err := commandHelp([]string{"record", "ios", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "launch_errors = 0") {
		t.Fatalf("expected ios help to mention launch error assertions, got:\n%s", text)
	}
	if !strings.Contains(text, "implicit zero-issue assertions cover launch errors, crashes, and fatal logs") {
		t.Fatalf("expected ios help to describe default ios health policy, got:\n%s", text)
	}
	if !strings.Contains(text, "`xcrun` detected on PATH") {
		t.Fatalf("expected ios help to include local simulator recommendations, got:\n%s", text)
	}
}

func TestRecordCLIHelpMentionsTranscriptAndScreenshotRequirements(t *testing.T) {
	withStubbedLookPath(t, "ghostty", "tmux")
	text, ok, err := commandHelp([]string{"record", "cli", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"--command \"cli subcommand --flag\"",
		"--transcript PATH",
		"every cli scenario needs a transcript, at least one screenshot, and at least one passing assertion",
		"`ghostty` and `tmux` are detected on PATH",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected cli help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestCommandHelpSupportsDesktopNestedSubcommandsWithoutActiveRun(t *testing.T) {
	text, ok, err := commandHelp([]string{"record", "desktop", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "proctor record desktop") {
		t.Fatalf("expected desktop help text, got:\n%s", text)
	}
	if !strings.Contains(text, "--report /abs/path/desktop-report.json") {
		t.Fatalf("expected desktop help to include report flag example, got:\n%s", text)
	}
	if !strings.Contains(text, "crashes = 0") {
		t.Fatalf("expected desktop help to include crash assertions, got:\n%s", text)
	}
	if !strings.Contains(text, "implicit zero-issue assertions cover crashes and fatal logs") {
		t.Fatalf("expected desktop help to describe default desktop health policy, got:\n%s", text)
	}
}

func TestRootHelpMentionsDesktopWorkflow(t *testing.T) {
	text, ok, err := commandHelp([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"Typical desktop workflow",
		"--platform desktop",
		"--app-name \"Firefox\"",
		"peekaboo",
		"proctor record desktop --help",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected root help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestStartHelpMentionsDesktopPlatform(t *testing.T) {
	text, ok, err := commandHelp([]string{"start", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"--platform web|ios|cli|desktop",
		"--app-name TEXT",
		"window management, resize, and multi-monitor",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected start help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestNoteHelpDescribesForcingFunction(t *testing.T) {
	text, ok, err := commandHelp([]string{"note", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"proctor note - file a pre-test note BEFORE recording evidence",
		"--scenario ID",
		"--session SESSION",
		"--notes TEXT",
		"forcing function",
		"proctor record refuses to accept evidence when no pre-note exists",
		"including curl",
		"Multiple pre-notes per (scenario, session) are allowed",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected note help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestHelpTopicSupportsNote(t *testing.T) {
	text, err := topicHelp([]string{"note"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "proctor note - file a pre-test note BEFORE recording evidence") {
		t.Fatalf("expected note topic help, got:\n%s", text)
	}
}

func TestRootHelpMentionsNoteStep(t *testing.T) {
	text, ok, err := commandHelp([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	for _, needle := range []string{
		"proctor note",
		"BEFORE recording",
		"proctor note --help",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected root help to mention %q, got:\n%s", needle, text)
		}
	}
}

func TestRecordHelpMentionsPreNoteGate(t *testing.T) {
	text, ok, err := commandHelp([]string{"record", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "every record call BLOCKS until a pre-test note has been filed") {
		t.Fatalf("expected record help to describe the pre-note gate, got:\n%s", text)
	}
}

func TestDoneHelpMentionsPreNoteGate(t *testing.T) {
	text, ok, err := commandHelp([]string{"done", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "at least one pre-test note filed") {
		t.Fatalf("expected done help to describe the pre-note gate, got:\n%s", text)
	}
}

func TestStatusHelpMentionsPreNoteVisibility(t *testing.T) {
	text, ok, err := commandHelp([]string{"status", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected help to be handled")
	}
	if !strings.Contains(text, "whether a pre-test note has been filed for each scenario") {
		t.Fatalf("expected status help to surface pre-note visibility, got:\n%s", text)
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
		"--platform web|ios|cli|desktop",
		"--cli-command TEXT",
		"--curl required|scenario|skip",
		`--curl-endpoint "happy-path=POST /api/login"`,
		"require curl only for named risky scenarios",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected start help to mention %q, got:\n%s", needle, text)
		}
	}
}

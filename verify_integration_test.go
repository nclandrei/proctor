package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nclandrei/proctor/internal/proctor"
)

// sharedVerifyBinaryPath holds the path to a proctor binary compiled once
// per `go test` process and reused across every TestVerifyFlow_* test below.
// Each test still gets its own PROCTOR_HOME + repo tempdir, so they stay
// independent even though they share the compiled binary.
var (
	sharedVerifyBinaryOnce sync.Once
	sharedVerifyBinaryPath string
)

// verifyIntegrationBinary compiles the proctor binary once per process via
// the existing buildProctorBinary helper (from cli_integration_test.go) and
// returns the path. Subsequent calls return the same path.
func verifyIntegrationBinary(t *testing.T) string {
	t.Helper()
	sharedVerifyBinaryOnce.Do(func() {
		proctorRoot, err := os.Getwd()
		if err != nil {
			t.Fatalf("resolve proctor root: %v", err)
		}
		dir, err := os.MkdirTemp("", "proctor-verify-binary-*")
		if err != nil {
			t.Fatalf("create binary tempdir: %v", err)
		}
		binaryPath := filepath.Join(dir, "proctor")
		buildProctorBinary(t, proctorRoot, binaryPath)
		sharedVerifyBinaryPath = binaryPath
	})
	if sharedVerifyBinaryPath == "" {
		t.Fatal("proctor binary not available")
	}
	return sharedVerifyBinaryPath
}

// verifyFixture wires up a proctor home + repo + binary for a single test and
// returns the paths the test will use for exec calls. Every test gets its own
// isolated filesystem state so they never interact.
type verifyFixture struct {
	binary        string
	proctorHome   string
	repoRoot      string
	happyShot     string
	failureShot   string
	happyScript   string
	failureScript string
}

func newVerifyFixture(t *testing.T) *verifyFixture {
	t.Helper()
	binary := verifyIntegrationBinary(t)
	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-verify-integration-test")

	fx := &verifyFixture{
		binary:        binary,
		proctorHome:   proctorHome,
		repoRoot:      repoRoot,
		happyShot:     writeIntegrationScreenshot(t, repoRoot, "verify-happy.png", "verify-happy-image"),
		failureShot:   writeIntegrationScreenshot(t, repoRoot, "verify-failure.png", "verify-failure-image"),
		happyScript:   writeIntegrationFixture(t, repoRoot, "verify-happy.txt", "Usage:\n  demo help\nonboarding prompt"),
		failureScript: writeIntegrationFixture(t, repoRoot, "verify-failure.txt", "error: prompt not found"),
	}
	return fx
}

// startCLIRun performs proctor start for a 2-scenario CLI run (happy + failure
// paths only, with all edge-case categories marked N/A). Callers use this to
// get to a recordable state quickly.
func (fx *verifyFixture) startCLIRun(t *testing.T) {
	t.Helper()
	startArgs := []string{
		"start",
		"--platform", "cli",
		"--feature", "verify integration",
		"--cli-command", "demo help",
		"--happy-path", "help output is readable",
		"--failure-path", "unknown argument fails clearly",
	}
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformCLI) {
		startArgs = append(startArgs, "--edge-case", category+"=N/A: covered by verify integration test")
	}
	runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, startArgs...)
}

// filePreNoteCLI files a pre-test note for the given scenario+session so the
// subsequent record call is accepted.
func (fx *verifyFixture) filePreNoteCLI(t *testing.T, scenarioID, sessionID string) {
	t.Helper()
	runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"note",
		"--scenario", scenarioID,
		"--session", sessionID,
		"--notes", "about to verify the "+scenarioID+" cli scenario with the fixture screenshot and transcript",
	)
}

// logStepAllScenariosCLI logs a verification step for each scenario via the
// proctor log CLI command. This satisfies the log gate for proctor done.
func (fx *verifyFixture) logStepAllScenariosCLI(t *testing.T, sessionID string, scenarios []string) {
	t.Helper()
	for _, scenarioID := range scenarios {
		screenshot := fx.happyShot
		if scenarioID == "failure-path" {
			screenshot = fx.failureShot
		}
		runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome,
			"log",
			"--scenario", scenarioID,
			"--session", sessionID,
			"--surface", "cli",
			"--screenshot", screenshot,
			"--action", "executed the demo help command for "+scenarioID+" scenario verification",
			"--observation", "terminal shows the "+scenarioID+" output with the expected text and no errors visible",
			"--comparison", "output matches the "+scenarioID+" scenario requirements as defined in the contract",
		)
	}
}

// recordCLIScenario records a CLI scenario using the fixture's pre-written
// screenshot+transcript pair. The scenario must already exist in the current
// run (i.e. "happy-path" or "failure-path").
func (fx *verifyFixture) recordCLIScenario(t *testing.T, scenarioID, sessionID string) {
	t.Helper()
	screenshot := fx.happyShot
	transcript := fx.happyScript
	command := "demo help"
	exitCode := "0"
	assertion := "output contains onboarding"
	if scenarioID == "failure-path" {
		screenshot = fx.failureShot
		transcript = fx.failureScript
		command = "demo help missing"
		exitCode = "2"
		assertion = "output contains prompt not found"
	}
	fx.filePreNoteCLI(t, scenarioID, sessionID)
	runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"record", "cli",
		"--scenario", scenarioID,
		"--session", sessionID,
		"--command", command,
		"--transcript", transcript,
		"--screenshot", "terminal="+screenshot,
		"--exit-code", exitCode,
		"--assert", assertion,
		"--assert", "exit_code = "+exitCode,
		"--assert", "screenshot = true",
	)
}

// recordAllCLIScenarios records the happy-path and failure-path scenarios with
// the supplied session id.
func (fx *verifyFixture) recordAllCLIScenarios(t *testing.T, sessionID string) {
	t.Helper()
	fx.recordCLIScenario(t, "happy-path", sessionID)
	fx.recordCLIScenario(t, "failure-path", sessionID)
}

// runProctorCLIExpectFail runs the proctor binary and expects a non-zero exit
// code. It returns the combined stdout+stderr output along with the exit error
// so tests can assert on the error message body.
func runProctorCLIExpectFail(t *testing.T, binary, repoRoot, proctorHome string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "PROCTOR_HOME="+proctorHome)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("%s %s unexpectedly succeeded:\n%s", binary, strings.Join(args, " "), out)
	}
	return string(out), err
}

// verifyAllScenariosCLI runs `proctor verify` for every scenario id in
// scenarios using the shared session id. Verifications are scenario-specific but
// always long enough to pass the MinVerificationLength gate and include a
// judgment word.
func verifyAllScenariosCLI(t *testing.T, binary, repoRoot, proctorHome, sessionID string, scenarios []string) {
	t.Helper()
	for _, scenarioID := range scenarios {
		verification := "This satisfies the " + scenarioID + " contract because the terminal shows the expected output clearly with no unexpected stack traces"
		runProctorCLI(t, binary, repoRoot, proctorHome,
			"verify",
			"--scenario", scenarioID,
			"--session", sessionID,
			"--verification", verification,
		)
	}
}

func TestVerifyFlow_HappyPath(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordAllCLIScenarios(t, "verify-session-1")

	fx.logStepAllScenariosCLI(t, "verify-session-1", []string{"happy-path", "failure-path"})
	verifyAllScenariosCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "verify-session-1", []string{"happy-path", "failure-path"})

	doneOutput := runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "done")
	if !strings.Contains(doneOutput, "PASS") {
		t.Fatalf("expected done to report PASS, got:\n%s", doneOutput)
	}
	if !strings.Contains(doneOutput, "Report:") {
		t.Fatalf("expected done output to mention Report path, got:\n%s", doneOutput)
	}
}

func TestVerifyFlow_DoneBlocksWithoutVerify(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordAllCLIScenarios(t, "no-verify-session")

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome, "done")
	if !strings.Contains(output, "FAIL") {
		t.Fatalf("expected done output to include FAIL, got:\n%s", output)
	}
	if !strings.Contains(output, "awaiting verification") {
		t.Fatalf("expected done output to mention awaiting verification, got:\n%s", output)
	}
	for _, scenarioID := range []string{"happy-path", "failure-path"} {
		if !strings.Contains(output, scenarioID) {
			t.Fatalf("expected done output to mention scenario %q, got:\n%s", scenarioID, output)
		}
	}
	if !strings.Contains(output, "verification contract incomplete") {
		t.Fatalf("expected done error to mention verification contract incomplete, got:\n%s", output)
	}
}

func TestVerifyFlow_EmptyVerificationRejected(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordCLIScenario(t, "happy-path", "empty-verification-session")

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"verify",
		"--scenario", "happy-path",
		"--session", "empty-verification-session",
		"--verification", "",
	)
	if !strings.Contains(output, "--verification") {
		t.Fatalf("expected empty --verification rejection to mention --verification flag, got:\n%s", output)
	}
}

func TestVerifyFlow_ShortVerificationRejected(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordCLIScenario(t, "happy-path", "short-verification-session")

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"verify",
		"--scenario", "happy-path",
		"--session", "short-verification-session",
		"--verification", "ok",
	)
	if !strings.Contains(output, "must be specific") {
		t.Fatalf("expected quality rejection to mention specificity rule, got:\n%s", output)
	}
	if !strings.Contains(output, "got 2 chars") {
		t.Fatalf("expected rejection to report observed char count, got:\n%s", output)
	}
}

func TestVerifyFlow_UnknownScenarioRejected(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordCLIScenario(t, "happy-path", "unknown-scenario-session")

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"verify",
		"--scenario", "doesnt-exist",
		"--session", "unknown-scenario-session",
		"--verification", "This satisfies the contract because the text is long enough to clear the minimum verification length gate",
	)
	if !strings.Contains(output, "no evidence for scenario") {
		t.Fatalf("expected unknown-scenario rejection to mention missing evidence, got:\n%s", output)
	}
	if !strings.Contains(output, "doesnt-exist") {
		t.Fatalf("expected rejection to name the unknown scenario, got:\n%s", output)
	}
}

func TestVerifyFlow_DoubleVerifyRejected(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordCLIScenario(t, "happy-path", "double-verify-session")

	verifyAllScenariosCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "double-verify-session", []string{"happy-path"})

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"verify",
		"--scenario", "happy-path",
		"--session", "double-verify-session",
		"--verification", "This satisfies the contract because the second attempt at verifying the same evidence should not be allowed",
	)
	if !strings.Contains(output, "already verified") {
		t.Fatalf("expected double-verify rejection to mention already-verified, got:\n%s", output)
	}
}

func TestVerifyFlow_MultipleScenariosAllNeedVerify(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordAllCLIScenarios(t, "partial-verify-session")
	fx.logStepAllScenariosCLI(t, "partial-verify-session", []string{"happy-path", "failure-path"})

	// Verify only happy-path and confirm done still fails and names the
	// unverified failure-path scenario.
	verifyAllScenariosCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "partial-verify-session", []string{"happy-path"})

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome, "done")
	if !strings.Contains(output, "failure-path") {
		t.Fatalf("expected done output to name the unverified failure-path scenario, got:\n%s", output)
	}
	if !strings.Contains(output, "awaiting verification") {
		t.Fatalf("expected done output to mention awaiting verification, got:\n%s", output)
	}
	if strings.Contains(output, "happy-path") && !strings.Contains(output, "happy-path (cli)") {
		// happy-path may still appear in the scenario listing, but NOT
		// flagged as awaiting verification. We assert that the
		// awaiting-verification issue does not name happy-path by
		// scanning the lines that include it.
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "happy-path") && strings.Contains(line, "awaiting verification") {
				t.Fatalf("expected happy-path to be cleared of verification gap, got line:\n%s", line)
			}
		}
	}

	// Verify the remaining scenario and expect done to pass.
	verifyAllScenariosCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "partial-verify-session", []string{"failure-path"})

	doneOutput := runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "done")
	if !strings.Contains(doneOutput, "PASS") {
		t.Fatalf("expected done to pass once every scenario is verified, got:\n%s", doneOutput)
	}
}

func TestVerifyFlow_StatusShowsPendingVerifications(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordAllCLIScenarios(t, "status-pending-session")

	statusOutput := runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "status")
	for _, needle := range []string{
		"awaiting verification",
		"happy-path",
		"failure-path",
		"status-pending-session",
	} {
		if !strings.Contains(statusOutput, needle) {
			t.Fatalf("expected status output to include %q, got:\n%s", needle, statusOutput)
		}
	}
	if !strings.Contains(statusOutput, "Status: incomplete") {
		t.Fatalf("expected status to report incomplete, got:\n%s", statusOutput)
	}
}

func TestVerifyFlow_LedgerStillBindsScenario(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordCLIScenario(t, "happy-path", "ledger-session")
	fx.recordCLIScenario(t, "failure-path", "ledger-session")

	// Locate the copied screenshot under ~/.proctor/.../artifacts/cli/happy-path/
	// and tamper with its contents. The ledger stores a SHA over the
	// original bytes, so any overwrite must be detected downstream.
	runsRoot := filepath.Join(fx.proctorHome, "runs")
	artifactPath := findCLIScreenshotArtifact(t, runsRoot, "happy-path")
	tamperedContent := make([]byte, 10*1024+1)
	for i := range tamperedContent {
		tamperedContent[i] = 'X'
	}
	if err := os.WriteFile(artifactPath, tamperedContent, 0o644); err != nil {
		t.Fatalf("tamper with artifact: %v", err)
	}

	// Verifying the tampered scenario must still succeed at the verify
	// call itself (verify does not re-hash artifacts), but downstream
	// status/done must detect the SHA mismatch.
	verifyAllScenariosCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "ledger-session", []string{"happy-path", "failure-path"})

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome, "done")
	if !strings.Contains(output, "artifact hash mismatch") {
		t.Fatalf("expected done to detect tampered artifact via SHA mismatch, got:\n%s", output)
	}
	if !strings.Contains(output, "happy-path") {
		t.Fatalf("expected SHA-mismatch output to mention happy-path, got:\n%s", output)
	}
}

func TestVerifyFlow_EvidenceJSONLContainsNotesAndStatus(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)
	fx.recordCLIScenario(t, "happy-path", "jsonl-session")

	notes := "This satisfies the contract because the verified observation captured by the jsonl integration test confirms the happy path"
	runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"verify",
		"--scenario", "happy-path",
		"--session", "jsonl-session",
		"--verification", notes,
	)

	runsRoot := filepath.Join(fx.proctorHome, "runs")
	evidencePath := findEvidenceJSONL(t, runsRoot)
	data, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("read evidence.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected evidence.jsonl to contain both the recorded and verified entries, got %d:\n%s", len(lines), string(data))
	}

	// Find the latest entry for happy-path / jsonl-session — that should be
	// the verified entry.
	var latest proctor.Evidence
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev proctor.Evidence
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("parse evidence line %q: %v", line, err)
		}
		if ev.ScenarioID != "happy-path" {
			continue
		}
		if ev.Provenance.SessionID != "jsonl-session" {
			continue
		}
		latest = ev
		found = true
	}
	if !found {
		t.Fatalf("evidence.jsonl did not contain an entry for happy-path / jsonl-session:\n%s", string(data))
	}
	if latest.Status != proctor.EvidenceStatusComplete {
		t.Fatalf("expected latest entry status to be %q, got %q", proctor.EvidenceStatusComplete, latest.Status)
	}
	if latest.Notes != notes {
		t.Fatalf("expected latest entry notes to match, got %q", latest.Notes)
	}
	if latest.VerifiedAt == nil || latest.VerifiedAt.IsZero() {
		t.Fatalf("expected latest entry to carry a non-zero VerifiedAt timestamp")
	}
}

// recordCLIScenarioWithoutPreNote is like recordCLIScenario but skips the
// pre-note step. Used by tests that exercise the record gate.
func (fx *verifyFixture) recordCLIScenarioWithoutPreNote(t *testing.T, scenarioID, sessionID string) (string, error) {
	t.Helper()
	screenshot := fx.happyShot
	transcript := fx.happyScript
	command := "demo help"
	exitCode := "0"
	assertion := "output contains onboarding"
	if scenarioID == "failure-path" {
		screenshot = fx.failureShot
		transcript = fx.failureScript
		command = "demo help missing"
		exitCode = "2"
		assertion = "output contains prompt not found"
	}
	return runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"record", "cli",
		"--scenario", scenarioID,
		"--session", sessionID,
		"--command", command,
		"--transcript", transcript,
		"--screenshot", "terminal="+screenshot,
		"--exit-code", exitCode,
		"--assert", assertion,
		"--assert", "exit_code = "+exitCode,
		"--assert", "screenshot = true",
	)
}

func TestNoteFlow_RecordBlocksWithoutPreNote(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)

	output, _ := fx.recordCLIScenarioWithoutPreNote(t, "happy-path", "no-prenote-session")
	if !strings.Contains(output, "file a pre-test note first") {
		t.Fatalf("expected record to surface pre-note gate error, got:\n%s", output)
	}
	if !strings.Contains(output, "proctor note --scenario happy-path --session no-prenote-session") {
		t.Fatalf("expected record to include concrete proctor note hint, got:\n%s", output)
	}
}

func TestNoteFlow_DoneBlocksWithoutPreNote(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)

	// File + record a pre-note for happy-path only. Then force-inject
	// failure-path evidence via the CLI by filing its pre-note + recording,
	// verifying both, then deleting the failure-path pre-notes from the
	// ledger. This exercises the done gate directly.
	fx.recordAllCLIScenarios(t, "partial-prenote-session")
	verifyAllScenariosCLI(t, fx.binary, fx.repoRoot, fx.proctorHome, "partial-prenote-session", []string{"happy-path", "failure-path"})

	// Truncate notes.jsonl to 0 bytes so the done gate sees evidence with
	// no pre-note. This simulates a legacy run without pre-notes.
	runsRoot := filepath.Join(fx.proctorHome, "runs")
	var notesPath string
	err := filepath.Walk(runsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == "notes.jsonl" {
			notesPath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk runs dir: %v", err)
	}
	if notesPath == "" {
		t.Fatalf("no notes.jsonl found under %s", runsRoot)
	}
	if err := os.WriteFile(notesPath, []byte{}, 0o644); err != nil {
		t.Fatalf("truncate notes.jsonl: %v", err)
	}

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome, "done")
	if !strings.Contains(output, "FAIL") {
		t.Fatalf("expected done output to include FAIL, got:\n%s", output)
	}
	if !strings.Contains(output, "has evidence but no pre-test note recorded") {
		t.Fatalf("expected done output to mention missing pre-note, got:\n%s", output)
	}
}

func TestNoteFlow_EmptyPreNoteRejected(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"note",
		"--scenario", "happy-path",
		"--session", "empty-notes-session",
		"--notes", "",
	)
	if !strings.Contains(output, "missing required flags") && !strings.Contains(output, "--notes is required") {
		t.Fatalf("expected empty notes to be rejected, got:\n%s", output)
	}
}

func TestNoteFlow_ShortPreNoteRejected(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)

	output, _ := runProctorCLIExpectFail(t, fx.binary, fx.repoRoot, fx.proctorHome,
		"note",
		"--scenario", "happy-path",
		"--session", "short-notes-session",
		"--notes", "too short",
	)
	if !strings.Contains(output, "must describe") {
		t.Fatalf("expected short notes to surface length error, got:\n%s", output)
	}
}

func TestNoteFlow_MultiplePreNotesAppendToLog(t *testing.T) {
	fx := newVerifyFixture(t)
	fx.startCLIRun(t)

	for i := 0; i < 3; i++ {
		runProctorCLI(t, fx.binary, fx.repoRoot, fx.proctorHome,
			"note",
			"--scenario", "happy-path",
			"--session", "audit-session",
			"--notes", "about to verify the cli happy path with the fixture transcript and screenshot",
		)
	}

	runsRoot := filepath.Join(fx.proctorHome, "runs")
	var notesPath string
	err := filepath.Walk(runsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == "notes.jsonl" {
			notesPath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk runs dir: %v", err)
	}
	if notesPath == "" {
		t.Fatalf("no notes.jsonl found under %s", runsRoot)
	}
	data, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 pre-notes appended to log, got %d:\n%s", len(lines), string(data))
	}
	seen := map[string]bool{}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var note proctor.PreNote
		if err := json.Unmarshal([]byte(line), &note); err != nil {
			t.Fatalf("parse pre-note line %q: %v", line, err)
		}
		if !strings.HasPrefix(note.ID, "note_") {
			t.Fatalf("expected pre-note id to start with note_, got %q", note.ID)
		}
		if seen[note.ID] {
			t.Fatalf("duplicate pre-note id %s", note.ID)
		}
		seen[note.ID] = true
	}
}

// findCLIScreenshotArtifact walks the runs directory to locate the first CLI
// screenshot artifact recorded under the given scenario. Returns the absolute
// path on disk.
func findCLIScreenshotArtifact(t *testing.T, runsRoot, scenarioID string) string {
	t.Helper()
	var found string
	err := filepath.Walk(runsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.Contains(path, filepath.Join("artifacts", "cli", scenarioID)) {
			return nil
		}
		if !strings.HasPrefix(filepath.Base(path), "terminal-") {
			return nil
		}
		if !strings.HasSuffix(path, ".png") {
			return nil
		}
		found = path
		return filepath.SkipAll
	})
	if err != nil {
		t.Fatalf("walk runs dir: %v", err)
	}
	if found == "" {
		t.Fatalf("no CLI screenshot artifact found under %s for scenario %s", runsRoot, scenarioID)
	}
	return found
}

// findEvidenceJSONL returns the path to the single evidence.jsonl under the
// runs tree.
func findEvidenceJSONL(t *testing.T, runsRoot string) string {
	t.Helper()
	var found string
	err := filepath.Walk(runsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "evidence.jsonl" {
			return nil
		}
		found = path
		return filepath.SkipAll
	})
	if err != nil {
		t.Fatalf("walk runs dir: %v", err)
	}
	if found == "" {
		t.Fatalf("no evidence.jsonl found under %s", runsRoot)
	}
	return found
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nclandrei/proctor/internal/proctor"
)

func TestCLIFlowViaGoRun(t *testing.T) {
	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-integration-test")

	terminalShotHappy := writeIntegrationScreenshot(t, repoRoot, "terminal-happy.png", "terminal-image-happy")
	terminalShotFailure := writeIntegrationScreenshot(t, repoRoot, "terminal-failure.png", "terminal-image-failure")
	happyTranscript := writeIntegrationFixture(t, repoRoot, "happy-pane.txt", "Usage:\n  demo help\nonboarding prompt")
	failureTranscript := writeIntegrationFixture(t, repoRoot, "failure-pane.txt", "error: prompt not found")

	startArgs := []string{
		"start",
		"--platform", "cli",
		"--feature", "cli integration flow",
		"--cli-command", "demo help",
		"--happy-path", "help output is readable",
		"--failure-path", "unknown argument fails clearly",
	}
	for _, edgeCase := range cliIntegrationNAEdgeCases() {
		startArgs = append(startArgs, "--edge-case", edgeCase)
	}
	runProctorCLI(t, proctorBinary, repoRoot, proctorHome, startArgs...)

	runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"note",
		"--scenario", "happy-path",
		"--session", "integration-cli-1",
		"--notes", "about to verify the demo help command prints usage and the onboarding prompt banner",
	)
	runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"note",
		"--scenario", "failure-path",
		"--session", "integration-cli-1",
		"--notes", "about to verify the demo help missing subcommand exits non-zero and prints a clear error",
	)

	runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"record", "cli",
		"--scenario", "happy-path",
		"--session", "integration-cli-1",
		"--command", "demo help",
		"--transcript", happyTranscript,
		"--screenshot", "terminal="+terminalShotHappy,
		"--exit-code", "0",
		"--assert", "output contains onboarding",
		"--assert", "exit_code = 0",
		"--assert", "screenshot = true",
	)

	runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"record", "cli",
		"--scenario", "failure-path",
		"--session", "integration-cli-1",
		"--command", "demo help missing",
		"--transcript", failureTranscript,
		"--screenshot", "terminal="+terminalShotFailure,
		"--exit-code", "2",
		"--assert", "output contains prompt not found",
		"--assert", "exit_code = 2",
		"--assert", "screenshot = true",
	)

	runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"verify",
		"--scenario", "happy-path",
		"--session", "integration-cli-1",
		"--notes", "terminal shows the demo help usage block and the onboarding prompt heading",
	)
	runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"verify",
		"--scenario", "failure-path",
		"--session", "integration-cli-1",
		"--notes", "terminal shows error: prompt not found with no stack trace and a non-zero exit code",
	)

	for _, scenario := range []string{"happy-path", "failure-path"} {
		shot := terminalShotHappy
		if scenario == "failure-path" {
			shot = terminalShotFailure
		}
		runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
			"log",
			"--scenario", scenario,
			"--session", "integration-cli-1",
			"--surface", "cli",
			"--screenshot", shot,
			"--action", "executed the demo command for "+scenario+" scenario verification",
			"--observation", "terminal shows the "+scenario+" output with the expected text and no errors visible",
			"--comparison", "output matches the "+scenario+" scenario requirements as defined in the contract",
		)
	}

	statusOutput := runProctorCLI(t, proctorBinary, repoRoot, proctorHome, "status")
	for _, needle := range []string{
		"Run: ",
		"happy-path",
		"cli: pass",
	} {
		if !strings.Contains(statusOutput, needle) {
			t.Fatalf("expected status output to include %q, got:\n%s", needle, statusOutput)
		}
	}

	doneOutput := runProctorCLI(t, proctorBinary, repoRoot, proctorHome, "done")
	if !strings.Contains(doneOutput, "PASS") {
		t.Fatalf("expected done output to pass, got:\n%s", doneOutput)
	}

	reportOutput := runProctorCLI(t, proctorBinary, repoRoot, proctorHome, "report")
	for _, needle := range []string{"Contract:", "HTML report:"} {
		if !strings.Contains(reportOutput, needle) {
			t.Fatalf("expected report output to include %q, got:\n%s", needle, reportOutput)
		}
	}
}

func runProctorCLI(t *testing.T, proctorBinary, repoRoot, proctorHome string, args ...string) string {
	t.Helper()
	cmd := exec.Command(proctorBinary, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "PROCTOR_HOME="+proctorHome)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", proctorBinary, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func buildProctorBinary(t *testing.T, proctorRoot, binaryPath string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = proctorRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

func initIntegrationGitRepo(t *testing.T, repoRoot, remote string) {
	t.Helper()
	runIntegrationCommand(t, repoRoot, "git", "init")
	runIntegrationCommand(t, repoRoot, "git", "config", "user.email", "test@example.com")
	runIntegrationCommand(t, repoRoot, "git", "config", "user.name", "Test User")
	runIntegrationCommand(t, repoRoot, "git", "remote", "add", "origin", remote)
}

func runIntegrationCommand(t *testing.T, dir string, command string, args ...string) {
	t.Helper()
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", command, args, err, out)
	}
}

func writeIntegrationFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeIntegrationScreenshot(t *testing.T, dir, name, content string) string {
	t.Helper()
	minSize := 10*1024 + 1
	// Prepend PNG magic bytes so the format check passes.
	padded := "\x89PNG\r\n\x1a\n" + content
	for len(padded) < minSize {
		padded += "\x00"
	}
	return writeIntegrationFixture(t, dir, name, padded)
}

func cliIntegrationNAEdgeCases() []string {
	inputs := make([]string, 0, len(proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformCLI)))
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformCLI) {
		inputs = append(inputs, category+"=N/A: covered by this integration test")
	}
	return inputs
}

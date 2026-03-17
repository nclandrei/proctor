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

	terminalShot := writeIntegrationFixture(t, repoRoot, "terminal.png", "terminal-image")
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
		"record", "cli",
		"--scenario", "happy-path",
		"--session", "integration-cli-1",
		"--command", "demo help",
		"--transcript", happyTranscript,
		"--screenshot", "terminal="+terminalShot,
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
		"--screenshot", "terminal="+terminalShot,
		"--exit-code", "2",
		"--assert", "output contains prompt not found",
		"--assert", "exit_code = 2",
		"--assert", "screenshot = true",
	)

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

func cliIntegrationNAEdgeCases() []string {
	inputs := make([]string, 0, len(proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformCLI)))
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformCLI) {
		inputs = append(inputs, category+"=N/A: covered by this integration test")
	}
	return inputs
}

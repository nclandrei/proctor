package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nclandrei/proctor/internal/proctor"
)

// integrationFakeCaptureBackend writes a deterministic PNG so the capture
// engine can produce a ledger entry without contacting real browsers or
// simulators during integration tests.
type integrationFakeCaptureBackend struct {
	content      []byte
	verification proctor.CaptureVerificationMode
}

func (b *integrationFakeCaptureBackend) Capture(ctx context.Context, dest proctor.CaptureDestination, opts proctor.CaptureOptions) (proctor.CaptureResult, error) {
	if err := os.WriteFile(dest.ArtifactPath, b.content, 0o644); err != nil {
		return proctor.CaptureResult{}, err
	}
	verification := b.verification
	if verification == "" {
		verification = proctor.CaptureVerifyNone
	}
	return proctor.CaptureResult{
		Target:       opts.Target,
		Verification: verification,
	}, nil
}

func integrationScreenshotBytes() []byte {
	minSize := 10*1024 + 1
	buf := make([]byte, minSize)
	copy(buf, []byte("png-capture-"))
	for i := len("png-capture-"); i < minSize; i++ {
		buf[i] = byte(i & 0xff)
	}
	return buf
}

func TestCaptureThenRecordBrowserRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-capture-roundtrip")

	store, err := proctor.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := proctor.CreateRun(store, repoRoot, proctor.StartOptions{
		Feature:        "capture roundtrip",
		BrowserURL:     "http://127.0.0.1:3000/login",
		CurlMode:       "skip",
		CurlSkipReason: "capture roundtrip test only",
		HappyPath:      "valid login redirects",
		FailurePath:    "invalid login shows error",
		EdgeCaseInputs: []string{
			"validation and malformed input=N/A: covered elsewhere",
			"empty or missing input=N/A: covered elsewhere",
			"retry or double-submit=N/A: covered elsewhere",
			"loading, latency, and race conditions=N/A: covered elsewhere",
			"network or server failure=N/A: covered elsewhere",
			"auth and session state=N/A: covered elsewhere",
			"refresh, back-navigation, and state persistence=N/A: covered elsewhere",
			"mobile or responsive behavior=N/A: covered elsewhere",
			"accessibility and keyboard behavior=N/A: covered elsewhere",
			"any feature-specific risks=N/A: covered elsewhere",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Swap in a fake capture backend for browser and restore afterwards.
	backend := &integrationFakeCaptureBackend{
		content:      integrationScreenshotBytes(),
		verification: proctor.CaptureVerifyMeta,
	}
	proctor.RegisterCaptureBackend(proctor.SurfaceBrowser, backend)
	t.Cleanup(func() {
		proctor.RegisterCaptureBackend(proctor.SurfaceBrowser, integrationRestoreStub{surface: proctor.SurfaceBrowser})
	})

	rec, err := proctor.Capture(context.Background(), store, run, proctor.CaptureOptions{
		Surface:    proctor.SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "browser-roundtrip-1",
		Label:      "desktop",
		Target:     map[string]string{"url": "http://127.0.0.1:3000/login"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !strings.HasPrefix(rec.ID, "cap_") {
		t.Fatalf("expected capture id, got %s", rec.ID)
	}
	if _, err := os.Stat(rec.ArtifactPath); err != nil {
		t.Fatalf("expected captured artifact on disk: %v", err)
	}

	report := writeIntegrationFixture(t, repoRoot, "report.json", `{
  "desktop": {"title": "Login", "finalUrl": "http://127.0.0.1:3000/dashboard", "issues": {"consoleErrors": 0, "consoleWarnings": 0, "pageErrors": 0, "failedRequests": 0, "httpErrors": 0}},
  "mobile":  {"title": "Login", "finalUrl": "http://127.0.0.1:3000/dashboard", "issues": {"consoleErrors": 0, "consoleWarnings": 0, "pageErrors": 0, "failedRequests": 0, "httpErrors": 0}}
}`)

	if err := proctor.RecordBrowser(store, run, proctor.BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-roundtrip-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": rec.ArtifactPath},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	}); err != nil {
		t.Fatalf("record browser after capture: %v", err)
	}
}

// integrationRestoreStub is a minimal stub used to restore the capture map
// after an integration test swaps in a fake backend. It mirrors the private
// stub in internal/proctor.
type integrationRestoreStub struct {
	surface string
}

func (s integrationRestoreStub) Capture(ctx context.Context, dest proctor.CaptureDestination, opts proctor.CaptureOptions) (proctor.CaptureResult, error) {
	return proctor.CaptureResult{}, &integrationStubError{surface: s.surface}
}

type integrationStubError struct{ surface string }

func (e *integrationStubError) Error() string {
	return "capture backend for " + e.surface + " not yet implemented"
}

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
	padded := content
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

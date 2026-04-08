package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nclandrei/proctor/internal/proctor"
)

func TestLogCLIIntegration(t *testing.T) {
	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-log-test")

	// Create start args for a web platform.
	startArgs := []string{
		"start",
		"--platform", "web",
		"--feature", "log integration test",
		"--url", "http://127.0.0.1:3000/login",
		"--curl", "skip",
		"--curl-skip-reason", "no backend for log test",
		"--happy-path", "login page loads correctly",
		"--failure-path", "invalid credentials show error",
	}
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformWeb) {
		startArgs = append(startArgs, "--edge-case", category+"=N/A: covered by log integration test")
	}
	runProctorCLI(t, proctorBinary, repoRoot, proctorHome, startArgs...)

	// Log step 1.
	shot1 := writeIntegrationScreenshot(t, repoRoot, "step1.png", "step1-login-page-content")
	output := runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "happy-path",
		"--session", "log-session-1",
		"--surface", "browser",
		"--screenshot", shot1,
		"--action", "Navigated to the login page and saw the login form with email and password fields",
	)
	if !strings.Contains(output, "Logged step 1") {
		t.Fatalf("expected step 1 output, got: %s", output)
	}
	if !strings.Contains(output, "happy-path") {
		t.Fatalf("expected scenario in output, got: %s", output)
	}

	// Log step 2.
	shot2 := writeIntegrationScreenshot(t, repoRoot, "step2.png", "step2-after-submit-content")
	output = runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "happy-path",
		"--session", "log-session-1",
		"--surface", "browser",
		"--screenshot", shot2,
		"--action", "Entered credentials and clicked the Submit button on the login form",
	)
	if !strings.Contains(output, "Logged step 2") {
		t.Fatalf("expected step 2 output, got: %s", output)
	}

	// Log step for a different scenario.
	shot3 := writeIntegrationScreenshot(t, repoRoot, "step3.png", "step3-failure-content")
	output = runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "failure-path",
		"--session", "log-session-2",
		"--surface", "browser",
		"--screenshot", shot3,
		"--action", "Entered wrong password and clicked Submit to test failure flow",
	)
	if !strings.Contains(output, "Logged step 1") {
		t.Fatalf("expected step 1 for new scenario, got: %s", output)
	}
	if !strings.Contains(output, "failure-path") {
		t.Fatalf("expected failure-path scenario in output, got: %s", output)
	}
}

func TestLogCLIRequiresFlags(t *testing.T) {
	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-log-req-test")

	// Create a run first.
	startArgs := []string{
		"start",
		"--platform", "web",
		"--feature", "log flag test",
		"--url", "http://127.0.0.1:3000/test",
		"--curl", "skip",
		"--curl-skip-reason", "test only",
		"--happy-path", "page loads",
		"--failure-path", "page fails",
	}
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformWeb) {
		startArgs = append(startArgs, "--edge-case", category+"=N/A: test only")
	}
	runProctorCLI(t, proctorBinary, repoRoot, proctorHome, startArgs...)

	// Try log without required flags - should fail.
	runProctorCLIExpectFail(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "happy-path",
	)
}

func TestAnalyzeCLIRequiresAPIKey(t *testing.T) {
	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-analyze-key-test")

	startArgs := []string{
		"start",
		"--platform", "web",
		"--feature", "analyze test",
		"--url", "http://127.0.0.1:3000/test",
		"--curl", "skip",
		"--curl-skip-reason", "test only",
		"--happy-path", "page loads",
		"--failure-path", "page fails",
	}
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformWeb) {
		startArgs = append(startArgs, "--edge-case", category+"=N/A: test only")
	}
	runProctorCLI(t, proctorBinary, repoRoot, proctorHome, startArgs...)

	// Analyze should fail without ANTHROPIC_API_KEY.
	t.Setenv("ANTHROPIC_API_KEY", "")
	output, _ := runProctorCLIExpectFail(t, proctorBinary, repoRoot, proctorHome,
		"analyze",
		"--scenario", "happy-path",
	)
	if !strings.Contains(output, "ANTHROPIC_API_KEY") {
		t.Fatalf("expected error mentioning ANTHROPIC_API_KEY, got: %s", output)
	}
}

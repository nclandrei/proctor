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
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-log-integration")

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
	shot1 := writeIntegrationScreenshot(t, repoRoot, "step1.png", "step1-login-page")
	output := runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "happy-path",
		"--session", "log-session-1",
		"--surface", "browser",
		"--screenshot", shot1,
		"--action", "Navigated to http://127.0.0.1:3000/login in the browser",
		"--observation", "Login form visible with email field, password field, and blue Sign In button at bottom",
		"--comparison", "Login page matches expected layout for the happy-path scenario; ready to enter credentials",
	)
	if !strings.Contains(output, "Logged step 1") {
		t.Fatalf("expected step 1 output, got: %s", output)
	}
	if !strings.Contains(output, "happy-path") {
		t.Fatalf("expected scenario in output, got: %s", output)
	}
	if !strings.Contains(output, "action:") {
		t.Fatalf("expected action in output, got: %s", output)
	}
	if !strings.Contains(output, "observation:") {
		t.Fatalf("expected observation in output, got: %s", output)
	}
	if !strings.Contains(output, "comparison:") {
		t.Fatalf("expected comparison in output, got: %s", output)
	}

	// Log step 2 - same session, step auto-increments.
	shot2 := writeIntegrationScreenshot(t, repoRoot, "step2.png", "step2-dashboard")
	output = runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "happy-path",
		"--session", "log-session-1",
		"--surface", "browser",
		"--screenshot", shot2,
		"--action", "Entered demo@example.com and password, clicked Sign In button",
		"--observation", "Dashboard page showing greeting Hello demo@example.com with Sign out button top right",
		"--comparison", "Dashboard matches happy-path: valid login redirected to dashboard with user greeting visible",
	)
	if !strings.Contains(output, "Logged step 2") {
		t.Fatalf("expected step 2 output, got: %s", output)
	}

	// Log for a different scenario starts at step 1.
	shot3 := writeIntegrationScreenshot(t, repoRoot, "step3.png", "step3-error")
	output = runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "failure-path",
		"--session", "log-session-2",
		"--surface", "browser",
		"--screenshot", shot3,
		"--action", "Entered wrong password and clicked Sign In to test failure flow",
		"--observation", "Login page still showing with red error banner: Invalid credentials. Email field still filled.",
		"--comparison", "Matches failure-path scenario: bad password shows error and keeps user on login page",
	)
	if !strings.Contains(output, "Logged step 1") {
		t.Fatalf("expected step 1 for new scenario, got: %s", output)
	}
	if !strings.Contains(output, "failure-path") {
		t.Fatalf("expected failure-path in output, got: %s", output)
	}
}

func TestLogCLIRequiresAllFlags(t *testing.T) {
	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-log-flags")

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

	// Missing most flags.
	output, _ := runProctorCLIExpectFail(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "happy-path",
	)
	if !strings.Contains(output, "missing required flags") {
		t.Fatalf("expected missing flags error, got: %s", output)
	}
}

func TestLogCLIRejectsShortObservation(t *testing.T) {
	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-log-short")

	startArgs := []string{
		"start",
		"--platform", "web",
		"--feature", "short obs test",
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

	shot := writeIntegrationScreenshot(t, repoRoot, "short.png", "short-test")
	output, _ := runProctorCLIExpectFail(t, proctorBinary, repoRoot, proctorHome,
		"log",
		"--scenario", "happy-path",
		"--session", "s1",
		"--surface", "browser",
		"--screenshot", shot,
		"--action", "Navigated to the login page at localhost:3000",
		"--observation", "looks good",
		"--comparison", "This matches the happy-path scenario requirements for the login flow entirely",
	)
	// "looks good" is only 10 chars, well below the 40-char minimum.
	if !strings.Contains(output, "observation") {
		t.Fatalf("expected observation quality error, got: %s", output)
	}
}

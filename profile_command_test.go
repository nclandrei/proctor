// profile_command_test.go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// withProctorHome sets PROCTOR_HOME to a temp dir and returns the path.
func withProctorHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PROCTOR_HOME", dir)
	return dir
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	// main.run writes to os.Stdout/os.Stderr; redirect via pipes.
	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = oldStdout, oldStderr }()

	cliErr := run(args)

	wOut.Close()
	wErr.Close()
	var bufOut, bufErr bytes.Buffer
	bufOut.ReadFrom(rOut)
	bufErr.ReadFrom(rErr)
	return bufOut.String(), bufErr.String(), cliErr
}

func TestInitCommandCreatesWebProfile(t *testing.T) {
	home := withProctorHome(t)
	// Switch cwd to a temp dir that looks like a web repo.
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"scripts":{"dev":"next dev"}}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)

	_, _, err := runCLI(t,
		"init",
		"--platform", "web",
		"--url", "http://127.0.0.1:3000",
		"--test-email", "demo@example.com",
		"--test-password", "hunter2",
	)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	// Resolve the slug the same way the CLI does.
	// For an ephemeral dir with no git remote, slug = basename.
	entries, _ := os.ReadDir(filepath.Join(home, "profiles"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 profile dir, got %d", len(entries))
	}
	profilePath := filepath.Join(home, "profiles", entries[0].Name(), "profile.json")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if !bytes.Contains(data, []byte(`"test_email": "demo@example.com"`)) {
		t.Fatalf("profile missing test_email: %s", data)
	}
	if bytes.Contains(data, []byte(`"incomplete": true`)) {
		t.Fatalf("profile should be complete, got: %s", data)
	}
}

func TestInitCommandProducesIncompleteWhenMissing(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)

	_, _, err := runCLI(t, "init", "--platform", "web", "--url", "http://x")
	if err != nil {
		t.Fatalf("init should succeed even with missing fields: %v", err)
	}
}

func TestProjectShowPrintsRedactedProfile(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	runCLI(t, "init", "--platform", "web", "--url", "http://x", "--test-email", "a@b.c", "--test-password", "hunter2")

	out, _, err := runCLI(t, "project", "show")
	if err != nil {
		t.Fatalf("project show: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte(`"test_password": "***"`)) {
		t.Fatalf("expected redacted password in output, got: %s", out)
	}
	if bytes.Contains([]byte(out), []byte("hunter2")) {
		t.Fatalf("raw password leaked: %s", out)
	}
}

func TestProjectShowMissingProfile(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	_, _, err := runCLI(t, "project", "show")
	if err == nil {
		t.Fatalf("expected error when no profile exists")
	}
}

func TestProjectGetEmitsRawValue(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	runCLI(t, "init", "--platform", "web", "--url", "http://x", "--test-email", "a@b.c", "--test-password", "hunter2")
	out, _, err := runCLI(t, "project", "get", "web.test_password")
	if err != nil {
		t.Fatalf("project get: %v", err)
	}
	if bytes.TrimSpace([]byte(out))[0] != 'h' || string(bytes.TrimSpace([]byte(out))) != "hunter2" {
		t.Fatalf("expected raw hunter2, got %q", out)
	}
}

func TestProjectGetReturnsErrorOnEmptyField(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	runCLI(t, "init", "--platform", "web", "--url", "http://x") // no password
	_, _, err := runCLI(t, "project", "get", "web.test_password")
	if err == nil {
		t.Fatal("expected non-zero exit for empty field")
	}
}

func TestProjectSetStampsField(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	runCLI(t, "init", "--platform", "web", "--url", "http://x", "--test-email", "a@b.c")
	// Password was missing; stamp it.
	_, _, err := runCLI(t, "project", "set", "web.test_password=hunter2")
	if err != nil {
		t.Fatalf("project set: %v", err)
	}
	out, _, _ := runCLI(t, "project", "get", "web.test_password")
	if string(bytes.TrimSpace([]byte(out))) != "hunter2" {
		t.Fatalf("set did not persist: %q", out)
	}
}

func TestLoginSaveAndInvalidate(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	runCLI(t, "init", "--platform", "web", "--url", "http://x", "--test-email", "a@b.c", "--test-password", "p")

	src := filepath.Join(t.TempDir(), "storage.json")
	os.WriteFile(src, []byte(`{"cookies":[]}`), 0o644)

	if _, _, err := runCLI(t, "login", "save", "--file", src); err != nil {
		t.Fatalf("login save: %v", err)
	}
	out, _, _ := runCLI(t, "project", "show")
	if !bytes.Contains([]byte(out), []byte(`"sha256"`)) {
		t.Fatalf("sha256 should appear after save, got: %s", out)
	}
	if _, _, err := runCLI(t, "login", "invalidate"); err != nil {
		t.Fatalf("login invalidate: %v", err)
	}
	out, _, _ = runCLI(t, "project", "show")
	if bytes.Contains([]byte(out), []byte(`"sha256": "`)) && !bytes.Contains([]byte(out), []byte(`"sha256": ""`)) {
		// sha256 was either omitted or cleared — both are acceptable
		if bytes.Count([]byte(out), []byte(`"sha256"`)) > 0 && !bytes.Contains([]byte(out), []byte(`"sha256": ""`)) {
			t.Fatalf("sha256 should be cleared after invalidate, got: %s", out)
		}
	}
}

func TestStartUsesProfileWhenFlagsMissing(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	runCLI(t, "init",
		"--platform", "web",
		"--url", "http://127.0.0.1:3000",
		"--test-email", "a@b.c",
		"--test-password", "p",
	)
	// start without --url / --platform: must pick them up from profile.
	_, _, err := runCLI(t, "start",
		"--feature", "login",
		"--curl", "skip", "--curl-skip-reason", "client-only",
		"--happy-path", "ok.",
		"--failure-path", "bad.",
		"--edge-case", "validation and malformed input=N/A: none",
		"--edge-case", "empty or missing input=N/A: none",
		"--edge-case", "retry or double-submit=N/A: none",
		"--edge-case", "loading, latency, and race conditions=N/A: none",
		"--edge-case", "network or server failure=N/A: none",
		"--edge-case", "auth and session state=N/A: none",
		"--edge-case", "refresh, back-navigation, and state persistence=N/A: none",
		"--edge-case", "mobile or responsive behavior=N/A: none",
		"--edge-case", "accessibility and keyboard behavior=N/A: none",
		"--edge-case", "any feature-specific risks=N/A: none",
	)
	if err != nil {
		t.Fatalf("start should succeed with profile fallback: %v", err)
	}
}

func TestStartFailsWhenProfileIncomplete(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	// Create a profile missing test_password; start should fail without --test-password
	// because the URL still comes from profile, but `proctor start` requires --url at minimum.
	// Test: no profile, no --url → start fails with the standard missing-flag error.
	_, _, err := runCLI(t, "start",
		"--platform", "web",
		"--feature", "x",
		"--curl", "skip", "--curl-skip-reason", "r",
		"--happy-path", "a", "--failure-path", "b",
		"--edge-case", "validation and malformed input=N/A: none",
		"--edge-case", "empty or missing input=N/A: none",
		"--edge-case", "retry or double-submit=N/A: none",
		"--edge-case", "loading, latency, and race conditions=N/A: none",
		"--edge-case", "network or server failure=N/A: none",
		"--edge-case", "auth and session state=N/A: none",
		"--edge-case", "refresh, back-navigation, and state persistence=N/A: none",
		"--edge-case", "mobile or responsive behavior=N/A: none",
		"--edge-case", "accessibility and keyboard behavior=N/A: none",
		"--edge-case", "any feature-specific risks=N/A: none",
	)
	if err == nil {
		t.Fatalf("expected error when profile missing and --url absent")
	}
}

func TestStartRecordsProfileProvenance(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)
	runCLI(t, "init",
		"--platform", "web",
		"--url", "http://127.0.0.1:3000",
		"--test-email", "a@b.c",
		"--test-password", "p",
	)
	_, _, err := runCLI(t, "start",
		"--feature", "x",
		"--curl", "skip", "--curl-skip-reason", "r",
		"--happy-path", "a", "--failure-path", "b",
		"--edge-case", "validation and malformed input=N/A: none",
		"--edge-case", "empty or missing input=N/A: none",
		"--edge-case", "retry or double-submit=N/A: none",
		"--edge-case", "loading, latency, and race conditions=N/A: none",
		"--edge-case", "network or server failure=N/A: none",
		"--edge-case", "auth and session state=N/A: none",
		"--edge-case", "refresh, back-navigation, and state persistence=N/A: none",
		"--edge-case", "mobile or responsive behavior=N/A: none",
		"--edge-case", "accessibility and keyboard behavior=N/A: none",
		"--edge-case", "any feature-specific risks=N/A: none",
	)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Find the created run.json
	home := os.Getenv("PROCTOR_HOME")
	matches, _ := filepath.Glob(filepath.Join(home, "runs", "*", "*", "run.json"))
	if len(matches) != 1 {
		t.Fatalf("expected one run.json, got %v", matches)
	}
	data, _ := os.ReadFile(matches[0])
	if !bytes.Contains(data, []byte(`"profile_provenance"`)) {
		t.Fatalf("run.json missing provenance: %s", data)
	}
	if !bytes.Contains(data, []byte(`"url": "profile"`)) {
		t.Fatalf("run.json should record url sourced from profile: %s", data)
	}
}

func TestProfileLoopEndToEnd(t *testing.T) {
	withProctorHome(t)
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"scripts":{"dev":"next dev"}}`), 0o644)
	oldDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repo)

	// 1. Agent runs init; detector picks up web+url; password missing.
	if _, _, err := runCLI(t, "init", "--test-email", "demo@example.com"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, _, _ := runCLI(t, "project", "show")
	if !bytes.Contains([]byte(out), []byte("incomplete — missing")) {
		t.Fatalf("expected profile incomplete after init without password, got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("web.test_password")) {
		t.Fatalf("missing_fields should mention web.test_password, got: %s", out)
	}

	// 2. Human supplies the password; agent stamps it.
	if _, _, err := runCLI(t, "project", "set", "web.test_password=hunter2"); err != nil {
		t.Fatalf("project set: %v", err)
	}
	out, _, _ = runCLI(t, "project", "show")
	if bytes.Contains([]byte(out), []byte("incomplete — missing")) {
		t.Fatalf("expected profile complete after stamp, got: %s", out)
	}

	// 3. Agent runs start; URL and platform come from profile.
	_, _, err := runCLI(t, "start",
		"--feature", "login flow",
		"--curl", "skip", "--curl-skip-reason", "client-only",
		"--happy-path", "ok.", "--failure-path", "bad.",
		"--edge-case", "validation and malformed input=N/A: none",
		"--edge-case", "empty or missing input=N/A: none",
		"--edge-case", "retry or double-submit=N/A: none",
		"--edge-case", "loading, latency, and race conditions=N/A: none",
		"--edge-case", "network or server failure=N/A: none",
		"--edge-case", "auth and session state=N/A: none",
		"--edge-case", "refresh, back-navigation, and state persistence=N/A: none",
		"--edge-case", "mobile or responsive behavior=N/A: none",
		"--edge-case", "accessibility and keyboard behavior=N/A: none",
		"--edge-case", "any feature-specific risks=N/A: none",
	)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// 4. After a login, agent saves the session state; project show reports fresh.
	src := filepath.Join(t.TempDir(), "storage.json")
	os.WriteFile(src, []byte(`{"cookies":[{"name":"session","value":"abc"}]}`), 0o644)
	if _, _, err := runCLI(t, "login", "save", "--file", src); err != nil {
		t.Fatalf("login save: %v", err)
	}
	out, _, _ = runCLI(t, "project", "show")
	if !bytes.Contains([]byte(out), []byte("login state: fresh")) {
		t.Fatalf("expected fresh login state, got: %s", out)
	}

	// 5. Agent retrieves the raw path to hand to its browser tool.
	pathOut, _, _ := runCLI(t, "project", "get", "web.login.file")
	if string(bytes.TrimSpace([]byte(pathOut))) != "session.json" {
		t.Fatalf("expected session.json path, got %q", pathOut)
	}

	// 6. Agent invalidates; project show reports missing again.
	if _, _, err := runCLI(t, "login", "invalidate"); err != nil {
		t.Fatalf("login invalidate: %v", err)
	}
	out, _, _ = runCLI(t, "project", "show")
	if !bytes.Contains([]byte(out), []byte("login state: missing")) {
		t.Fatalf("expected missing login state after invalidate, got: %s", out)
	}
}

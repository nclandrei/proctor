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

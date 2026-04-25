package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestVersionCommandPrintsVersion(t *testing.T) {
	binary := buildProctor(t)
	out, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version command failed: %v\n%s", err, out)
	}
	expected := "proctor " + version
	if !strings.Contains(string(out), expected) {
		t.Fatalf("expected %q, got %q", expected, string(out))
	}
}

func TestVersionFlagPrintsVersion(t *testing.T) {
	binary := buildProctor(t)
	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("--version flag failed: %v\n%s", err, out)
	}
	expected := "proctor " + version
	if !strings.Contains(string(out), expected) {
		t.Fatalf("expected %q, got %q", expected, string(out))
	}
}

func TestVersionLdflagsInjection(t *testing.T) {
	dir := t.TempDir()
	binary := dir + "/proctor"
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version=42.0.1", "-o", binary, ".")
	cmd.Dir = projectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	out, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version command failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "proctor 42.0.1") {
		t.Fatalf("expected injected version 42.0.1, got %q", string(out))
	}
}

func buildProctor(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binary := dir + "/proctor"
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = projectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return binary
}

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

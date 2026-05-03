// internal/proctor/secrets_test.go
package proctor

import (
	"os"
	"strings"
	"testing"
)

func TestResolveSecretLiteral(t *testing.T) {
	got, err := ResolveSecret("hunter2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hunter2" {
		t.Fatalf("got %q want %q", got, "hunter2")
	}
}

func TestResolveSecretEmpty(t *testing.T) {
	got, err := ResolveSecret("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestResolveSecretEnvSet(t *testing.T) {
	t.Setenv("PROCTOR_TEST_SECRET", "fromenv")
	got, err := ResolveSecret("env:PROCTOR_TEST_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fromenv" {
		t.Fatalf("got %q want %q", got, "fromenv")
	}
}

func TestResolveSecretEnvUnset(t *testing.T) {
	os.Unsetenv("PROCTOR_TEST_SECRET_UNSET")
	_, err := ResolveSecret("env:PROCTOR_TEST_SECRET_UNSET")
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
	if !strings.Contains(err.Error(), "PROCTOR_TEST_SECRET_UNSET") {
		t.Fatalf("error should name the missing var, got: %v", err)
	}
}

func TestResolveSecretEnvEmptyName(t *testing.T) {
	_, err := ResolveSecret("env:")
	if err == nil {
		t.Fatal("expected error for empty env var name")
	}
}

func TestResolveSecretOpRefMissingBinary(t *testing.T) {
	// Force op lookups through a PATH that has no `op` binary.
	t.Setenv("PATH", t.TempDir())
	_, err := ResolveSecret("op://Private/MyApp/password")
	if err == nil {
		t.Fatal("expected error when op binary is unavailable")
	}
	if !strings.Contains(err.Error(), "op") {
		t.Fatalf("error should mention op, got: %v", err)
	}
}

func TestResolveSecretOpRefSuccess(t *testing.T) {
	// Stub `op` binary that echoes the secret.
	dir := t.TempDir()
	stub := dir + "/op"
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho stub-secret\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir)

	got, err := ResolveSecret("op://Private/MyApp/password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "stub-secret" {
		t.Fatalf("got %q want %q", got, "stub-secret")
	}
}

func TestResolveSecretOpRefFailure(t *testing.T) {
	dir := t.TempDir()
	stub := dir + "/op"
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho \"not found\" >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir)

	_, err := ResolveSecret("op://Private/MyApp/password")
	if err == nil {
		t.Fatal("expected error when op exits non-zero")
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nclandrei/proctor/internal/proctor"
)

func TestRunReportRegeneratesReportFiles(t *testing.T) {
	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	store, err := proctor.NewStore()
	if err != nil {
		t.Fatal(err)
	}

	run, err := proctor.CreateRun(store, repoRoot, testStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	reportPath := filepath.Join(store.RunDir(run), "report.html")
	if err := os.WriteFile(reportPath, []byte("stale report"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runReport(store, repoRoot); err != nil {
		t.Fatal(err)
	}

	report, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	if strings.Contains(text, "stale report") {
		t.Fatalf("expected runReport to regenerate report, got stale contents:\n%s", text)
	}
	if !strings.Contains(text, `<meta name="color-scheme" content="light">`) {
		t.Fatalf("expected regenerated report to set color-scheme, got:\n%s", text)
	}
}

func TestRunReportDoesNotMutateRunStatus(t *testing.T) {
	proctorHome := t.TempDir()
	t.Setenv("PROCTOR_HOME", proctorHome)

	repoRoot := t.TempDir()
	store, err := proctor.NewStore()
	if err != nil {
		t.Fatal(err)
	}

	run, err := proctor.CreateRun(store, repoRoot, testStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	// Run is freshly created with no evidence — status should be in_progress.
	before, err := store.LoadRun(proctor.RepoRoot(repoRoot))
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != "in_progress" {
		t.Fatalf("expected initial status in_progress, got %q", before.Status)
	}

	// Running proctor report should regenerate reports but not touch status.
	if err := runReport(store, repoRoot); err != nil {
		t.Fatal(err)
	}

	after, err := store.LoadRun(proctor.RepoRoot(repoRoot))
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "in_progress" {
		t.Fatalf("expected proctor report to preserve in_progress status, got %q", after.Status)
	}

	_ = run
}

func testStartOptions() proctor.StartOptions {
	inputs := make([]string, 0, len(proctor.EdgeCaseCategories))
	for _, category := range proctor.EdgeCaseCategories {
		inputs = append(inputs, category+"=N/A: test coverage not needed")
	}
	return proctor.StartOptions{
		Feature:        "report regeneration",
		BrowserURL:     "http://127.0.0.1:3000/example",
		CurlMode:       "skip",
		CurlSkipReason: "No backend contract for this test",
		HappyPath:      "Happy path",
		FailurePath:    "Failure path",
		EdgeCaseInputs: inputs,
	}
}

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
	if !strings.Contains(text, `<meta name="color-scheme" content="dark">`) {
		t.Fatalf("expected regenerated report to force dark mode, got:\n%s", text)
	}
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

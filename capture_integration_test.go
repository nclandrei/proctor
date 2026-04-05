package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// captureIntegrationChromeAvailable returns true if the current machine
// has a Chrome or Chromium binary resolvable the same way
// internal/proctor's browser backend resolves it. Tests that need a real
// browser capture skip when this returns false.
func captureIntegrationChromeAvailable() bool {
	if override := strings.TrimSpace(os.Getenv("PROCTOR_CHROME")); override != "" {
		if _, err := os.Stat(override); err == nil {
			return true
		}
	}
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return true
		}
	}
	for _, name := range []string{
		"google-chrome-stable",
		"google-chrome",
		"chromium",
		"chromium-browser",
	} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// startCaptureTestServer boots a tiny HTTP server that returns an HTML
// page rich enough for Chrome's PNG output to exceed proctor's 10KiB
// minimum-screenshot-size check. The response renders a grid of colored
// tiles so the compressed screenshot has enough entropy to pass.
func startCaptureTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	var body strings.Builder
	body.WriteString(`<!DOCTYPE html><html><head><title>Proctor Test</title><style>
body{margin:0;background:#fff;font-family:sans-serif}
.grid{display:grid;grid-template-columns:repeat(12,1fr);gap:2px;padding:8px}
.tile{padding:12px;color:#fff;font-weight:700;text-align:center}
</style></head><body><h1>Proctor Capture Test Page</h1><div class="grid">`)
	colors := []string{
		"#e74c3c", "#3498db", "#2ecc71", "#f1c40f", "#9b59b6", "#1abc9c",
		"#e67e22", "#34495e", "#16a085", "#c0392b", "#27ae60", "#8e44ad",
	}
	for i := 0; i < 72; i++ {
		c := colors[i%len(colors)]
		body.WriteString(`<div class="tile" style="background:` + c + `">`)
		body.WriteString("Tile ")
		body.WriteString(strconv.Itoa(i))
		body.WriteString("</div>")
	}
	body.WriteString(`</div><p>This page exists solely to produce a PNG large enough for proctor's ` +
		`minimum screenshot size validation. Its content is deterministic so the ` +
		`captured SHA256 is reproducible across runs.</p></body></html>`)
	rendered := body.String()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(rendered))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// parseCaptureStdout parses the `captured:` / `artifact:` / `sha256:`
// three-line block emitted by `proctor capture` and returns the parsed
// (captureID, artifactPath, sha256) triple.
func parseCaptureStdout(t *testing.T, stdout string) (captureID, artifactPath, sha256 string) {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "captured: "):
			captureID = strings.TrimSpace(strings.TrimPrefix(line, "captured: "))
		case strings.HasPrefix(line, "artifact: "):
			artifactPath = strings.TrimSpace(strings.TrimPrefix(line, "artifact: "))
		case strings.HasPrefix(line, "sha256: "):
			sha256 = strings.TrimSpace(strings.TrimPrefix(line, "sha256: "))
		}
	}
	if captureID == "" || artifactPath == "" || sha256 == "" {
		t.Fatalf("could not parse capture stdout:\n%s", stdout)
	}
	return
}

// webIntegrationNAEdgeCases builds --edge-case flags that mark every web
// category as N/A so CreateRun accepts the contract without additional
// scenario work.
func webIntegrationNAEdgeCases() []string {
	return []string{
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
	}
}

// writeCaptureBrowserReport writes a browser report JSON fixture targeting
// the given finalURL with no issues.
func writeCaptureBrowserReport(t *testing.T, dir, name, finalURL string) string {
	t.Helper()
	body := `{
  "desktop": {"title": "Proctor Test", "finalUrl": "` + finalURL + `", "issues": {"consoleErrors": 0, "consoleWarnings": 0, "pageErrors": 0, "failedRequests": 0, "httpErrors": 0}},
  "mobile":  {"title": "Proctor Test", "finalUrl": "` + finalURL + `", "issues": {"consoleErrors": 0, "consoleWarnings": 0, "pageErrors": 0, "failedRequests": 0, "httpErrors": 0}}
}`
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// locateActiveRunDir walks $PROCTOR_HOME/repos/*/active-run to find the
// currently-active run directory written by the proctor binary.
func locateActiveRunDir(t *testing.T, proctorHome string) string {
	t.Helper()
	reposRoot := filepath.Join(proctorHome, "repos")
	entries, err := os.ReadDir(reposRoot)
	if err != nil {
		t.Fatalf("read repos dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ptrPath := filepath.Join(reposRoot, e.Name(), "active-run")
		raw, err := os.ReadFile(ptrPath)
		if err != nil {
			continue
		}
		runDir := strings.TrimSpace(string(raw))
		if runDir != "" {
			return runDir
		}
	}
	t.Fatalf("could not locate active-run pointer under %s", reposRoot)
	return ""
}

// startCaptureRun bootstraps a web platform run for the capture integration
// tests via the proctor binary.
func startCaptureRun(t *testing.T, proctorBinary, repoRoot, proctorHome, featureLabel, browserURL string) {
	t.Helper()
	startArgs := []string{
		"start",
		"--platform", "web",
		"--feature", featureLabel,
		"--url", browserURL,
		"--curl", "skip",
		"--curl-skip-reason", "capture integration test only",
		"--happy-path", "proctor test page renders",
		"--failure-path", "proctor test page fails to render",
	}
	for _, edgeCase := range webIntegrationNAEdgeCases() {
		startArgs = append(startArgs, "--edge-case", edgeCase)
	}
	runProctorCLI(t, proctorBinary, repoRoot, proctorHome, startArgs...)
}

// TestCaptureIntegration_BinaryHappyPath drives the capture → record
// round-trip through the compiled proctor binary against a local httptest
// server. Chrome-gated: skips if no Chrome/Chromium binary is available.
func TestCaptureIntegration_BinaryHappyPath(t *testing.T) {
	if !captureIntegrationChromeAvailable() {
		t.Skip("no Chrome/Chromium available; skipping binary capture integration test")
	}

	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-capture-bin-happy")

	srv := startCaptureTestServer(t)
	startCaptureRun(t, proctorBinary, repoRoot, proctorHome, "capture binary happy", srv.URL)

	captureOut := runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"capture", "browser",
		"--scenario", "happy-path",
		"--session", "bin-happy-sess-1",
		"--label", "desktop",
		"--url", srv.URL,
	)
	captureID, artifactPath, sha := parseCaptureStdout(t, captureOut)
	if !strings.HasPrefix(captureID, "cap_") {
		t.Fatalf("expected cap_ prefix, got %q", captureID)
	}
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("capture artifact missing on disk: %v", err)
	}
	if len(sha) != 64 {
		t.Fatalf("expected 64-char SHA, got %q", sha)
	}

	report := writeCaptureBrowserReport(t, repoRoot, "report.json", srv.URL)

	runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"record", "browser",
		"--scenario", "happy-path",
		"--session", "bin-happy-sess-1",
		"--report", report,
		"--screenshot", "desktop="+artifactPath,
		"--assert", "console_errors = 0",
		"--capture-id", captureID,
	)

	// Verify evidence.jsonl contains exactly one browser record. Use the
	// active-run pointer so we locate the per-run directory regardless of
	// the on-disk slug layout.
	runDir := locateActiveRunDir(t, proctorHome)

	evidenceFile := filepath.Join(runDir, "evidence.jsonl")
	raw, err := os.ReadFile(evidenceFile)
	if err != nil {
		t.Fatalf("read evidence.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 evidence line, got %d:\n%s", len(lines), string(raw))
	}
	var evidence struct {
		Surface    string `json:"surface"`
		ScenarioID string `json:"scenario_id"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if evidence.Surface != "browser" || evidence.ScenarioID != "happy-path" {
		t.Fatalf("unexpected evidence contents: %+v", evidence)
	}

	// The captures.jsonl ledger should also contain exactly one record.
	captureFile := filepath.Join(runDir, "captures.jsonl")
	rawCap, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("read captures.jsonl: %v", err)
	}
	capLines := strings.Split(strings.TrimRight(string(rawCap), "\n"), "\n")
	if len(capLines) != 1 {
		t.Fatalf("expected 1 capture ledger line, got %d", len(capLines))
	}
	var ledgerRec struct {
		ID       string `json:"id"`
		Surface  string `json:"surface"`
		Scenario string `json:"scenario_id"`
		SHA      string `json:"artifact_sha256"`
	}
	if err := json.Unmarshal([]byte(capLines[0]), &ledgerRec); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	if ledgerRec.ID != captureID {
		t.Fatalf("ledger id %q vs capture id %q", ledgerRec.ID, captureID)
	}
	if ledgerRec.Surface != "browser" || ledgerRec.Scenario != "happy-path" {
		t.Fatalf("unexpected ledger contents: %+v", ledgerRec)
	}
	if ledgerRec.SHA != sha {
		t.Fatalf("ledger SHA %q vs stdout %q", ledgerRec.SHA, sha)
	}
}

// TestCaptureIntegration_BinaryTamperDetection drives a capture through
// the binary, overwrites the captured artifact, and verifies that
// `proctor record browser --capture-id` exits non-zero with a SHA
// mismatch error. Chrome-gated.
func TestCaptureIntegration_BinaryTamperDetection(t *testing.T) {
	if !captureIntegrationChromeAvailable() {
		t.Skip("no Chrome/Chromium available; skipping binary tamper integration test")
	}

	proctorRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	proctorBinary := filepath.Join(t.TempDir(), "proctor")
	buildProctorBinary(t, proctorRoot, proctorBinary)

	proctorHome := t.TempDir()
	repoRoot := t.TempDir()
	initIntegrationGitRepo(t, repoRoot, "https://github.com/nclandrei/proctor-capture-bin-tamper")

	srv := startCaptureTestServer(t)
	startCaptureRun(t, proctorBinary, repoRoot, proctorHome, "capture binary tamper", srv.URL)

	captureOut := runProctorCLI(t, proctorBinary, repoRoot, proctorHome,
		"capture", "browser",
		"--scenario", "happy-path",
		"--session", "bin-tamper-sess-1",
		"--label", "desktop",
		"--url", srv.URL,
	)
	captureID, artifactPath, _ := parseCaptureStdout(t, captureOut)

	// Overwrite the PNG with some other PNG-sized blob.
	tampered := make([]byte, 32*1024)
	copy(tampered, []byte("tampered-binary-blob"))
	for i := len("tampered-binary-blob"); i < len(tampered); i++ {
		tampered[i] = byte((i * 13) & 0xff)
	}
	if err := os.WriteFile(artifactPath, tampered, 0o644); err != nil {
		t.Fatal(err)
	}

	report := writeCaptureBrowserReport(t, repoRoot, "report.json", srv.URL)

	cmd := exec.Command(proctorBinary,
		"record", "browser",
		"--scenario", "happy-path",
		"--session", "bin-tamper-sess-1",
		"--report", report,
		"--screenshot", "desktop="+artifactPath,
		"--assert", "console_errors = 0",
		"--capture-id", captureID,
	)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "PROCTOR_HOME="+proctorHome)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected record to fail after tamper, got success: %s", out)
	}
	if !strings.Contains(string(out), "no submitted artifact matches capture") {
		t.Fatalf("expected SHA mismatch error in stderr, got:\n%s", out)
	}
}

package proctor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// e2eScreenshotBytes returns a valid-sized PNG payload that is unique per
// seed string.
func e2eScreenshotBytes(seed string) []byte {
	minSize := int(DefaultMinScreenshotSize) + 1
	buf := make([]byte, minSize)
	copy(buf, []byte("e2e-png-"+seed+"-"))
	for i := len("e2e-png-" + seed + "-"); i < minSize; i++ {
		buf[i] = byte((i + len(seed)) & 0xff)
	}
	return buf
}

// webRun creates a fresh web platform run with a distinct remote so the
// store paths are isolated.
func webRun(t *testing.T, remote string) (*Store, Run) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, remote)
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}
	return store, run
}

// e2eBrowserReport returns a browser report JSON fixture at the given repo
// path. It targets the provided finalURL and contains zero issues.
func e2eBrowserReport(t *testing.T, repo, name, finalURL string) string {
	t.Helper()
	body := `{
  "desktop": {"title": "Login", "finalUrl": "` + finalURL + `", "issues": {"consoleErrors": 0, "consoleWarnings": 0, "pageErrors": 0, "failedRequests": 0, "httpErrors": 0}},
  "mobile":  {"title": "Login", "finalUrl": "` + finalURL + `", "issues": {"consoleErrors": 0, "consoleWarnings": 0, "pageErrors": 0, "failedRequests": 0, "httpErrors": 0}}
}`
	path := filepath.Join(repo, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// e2eWriteScreenshot writes a unique PNG payload at repo/name and returns
// its absolute path. Content is seeded by name so tests can control SHA
// uniqueness.
func e2eWriteScreenshot(t *testing.T, repo, name, seed string) string {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.WriteFile(path, e2eScreenshotBytes(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCaptureE2E_LedgerAutoPopulated verifies that a successful
// RecordBrowser call appends one ledger entry per image artifact it
// accepts, that Evidence.CaptureIDs matches the ledger, and that the
// ledger SHAs match the submitted artifact SHAs.
func TestCaptureE2E_LedgerAutoPopulated(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-auto")

	desktop := e2eWriteScreenshot(t, run.RepoRoot, "desktop.png", "auto-desktop")
	mobile := e2eWriteScreenshot(t, run.RepoRoot, "mobile.png", "auto-mobile")
	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	filePreNote(t, store, run, "happy-path", "browser-e2e-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-e2e-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop, "mobile": mobile},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("record browser: %v", err)
	}

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(evidence))
	}
	ev := evidence[0]
	if len(ev.CaptureIDs) != 2 {
		t.Fatalf("expected 2 capture ids on evidence, got %d: %v", len(ev.CaptureIDs), ev.CaptureIDs)
	}

	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", len(records))
	}

	// Every capture id on the evidence resolves through the ledger.
	for _, id := range ev.CaptureIDs {
		if !strings.HasPrefix(id, "cap_") {
			t.Errorf("capture id missing cap_ prefix: %s", id)
		}
		rec, ok, err := store.CaptureLedger(run).FindByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("capture id %s missing from ledger", id)
		}
		if rec.Surface != SurfaceBrowser {
			t.Errorf("ledger record %s wrong surface: %s", id, rec.Surface)
		}
		if rec.ScenarioID != "happy-path" {
			t.Errorf("ledger record %s wrong scenario: %s", id, rec.ScenarioID)
		}
		if rec.SessionID != "browser-e2e-1" {
			t.Errorf("ledger record %s wrong session: %s", id, rec.SessionID)
		}
	}
}

// TestCaptureE2E_TamperedArtifactDetectedByLedger verifies that after a
// successful record, we can detect tampering by comparing the ledger's
// recorded SHA with the artifact that would-be submitted after tampering.
// verifyCaptureBinding remains available for callers who want to validate
// a submission against a ledger entry out-of-band.
func TestCaptureE2E_TamperedArtifactDetectedByLedger(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-tamper")

	desktop := e2eWriteScreenshot(t, run.RepoRoot, "desktop.png", "tamper-desktop")
	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	filePreNote(t, store, run, "happy-path", "tamper-1")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "tamper-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("record browser: %v", err)
	}

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || len(evidence[0].CaptureIDs) != 1 {
		t.Fatalf("expected 1 evidence with 1 capture id, got %#v", evidence)
	}
	captureID := evidence[0].CaptureIDs[0]

	// Look up the ledger record and verify its SHA matches the artifact
	// that landed in evidence.
	rec, ok, err := store.CaptureLedger(run).FindByID(captureID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("capture %s missing from ledger", captureID)
	}
	var imgSHA string
	for _, art := range evidence[0].Artifacts {
		if art.Kind == ArtifactImage {
			imgSHA = art.SHA256
			break
		}
	}
	if rec.ArtifactSHA256 != imgSHA {
		t.Fatalf("ledger sha %s does not match evidence sha %s", rec.ArtifactSHA256, imgSHA)
	}

	// Simulate a tampered submission via verifyCaptureBinding: hand it an
	// artifact whose SHA differs from the ledger entry, and it should
	// reject as "no submitted artifact matches capture".
	err = verifyCaptureBinding(store, run, "happy-path", "tamper-1", SurfaceBrowser, rec.ID, []Artifact{{SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}})
	if err == nil {
		t.Fatal("expected verifyCaptureBinding to reject tampered sha")
	}
	if !strings.Contains(err.Error(), "no submitted artifact matches capture") {
		t.Fatalf("expected tamper error, got %v", err)
	}
}

// TestCaptureE2E_CrossScenarioReuseRejected verifies that attempting to
// reuse the same screenshot SHA for a second scenario is rejected by the
// cross-scenario duplicate detector.
func TestCaptureE2E_CrossScenarioReuseRejected(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-reuse")

	desktop := e2eWriteScreenshot(t, run.RepoRoot, "desktop.png", "reuse-desktop")
	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	// First record binds the artifact to scenario happy-path.
	filePreNote(t, store, run, "happy-path", "reuse-sess")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "reuse-sess",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("first record: %v", err)
	}

	// Second record attempts to reuse the same file (identical content,
	// identical SHA) for a different scenario. detectDuplicateScreenshots
	// should reject it.
	filePreNote(t, store, run, "failure-path", "reuse-sess")
	err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "failure-path",
		SessionID:      "reuse-sess",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
	})
	if err == nil {
		t.Fatal("expected cross-scenario reuse to be rejected")
	}
	if !strings.Contains(err.Error(), "each scenario requires unique evidence") {
		t.Fatalf("expected duplicate-screenshot error, got %v", err)
	}

	// Ledger should still have one entry (only the successful record).
	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger entry after failed reuse, got %d", len(records))
	}
}

// TestCaptureE2E_ConcurrentRecordsLedgerConsistent spawns N parallel
// RecordBrowser goroutines (one per scenario to avoid duplicate-screenshot
// rejections) and verifies the ledger file ends well-formed with one
// entry per image artifact.
func TestCaptureE2E_ConcurrentRecordsLedgerConsistent(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-concurrent")

	// We need to use two distinct scenarios so cross-scenario dedupe
	// doesn't reject identical image bytes. Use both happy-path and
	// failure-path, each with unique screenshot content.
	scenarios := []string{"happy-path", "failure-path"}
	const perScenario = 1
	total := len(scenarios) * perScenario

	for _, scen := range scenarios {
		filePreNote(t, store, run, scen, fmt.Sprintf("conc-sess-%s", scen))
	}

	var wg sync.WaitGroup
	errs := make([]error, total)
	wg.Add(total)
	for idx, scen := range scenarios {
		idx := idx
		scen := scen
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs[idx] = fmt.Errorf("panic: %v", r)
				}
			}()
			screenshot := e2eWriteScreenshot(t, run.RepoRoot, fmt.Sprintf("concurrent-%s.png", scen), fmt.Sprintf("conc-%s", scen))
			report := e2eBrowserReport(t, run.RepoRoot, fmt.Sprintf("report-%s.json", scen), "http://127.0.0.1:3000/dashboard")
			if err := RecordBrowser(store, run, BrowserRecordOptions{
				ScenarioID:     scen,
				SessionID:      fmt.Sprintf("conc-sess-%s", scen),
				ReportPath:     report,
				Screenshots:    map[string]string{"desktop": screenshot},
				PassAssertions: []string{"console_errors = 0"},
			}); err != nil {
				errs[idx] = err
			}
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	// Ledger file should contain `total` JSONL lines, each a valid
	// CaptureRecord with a unique id.
	ledgerPath := filepath.Join(store.RunDir(run), "captures.jsonl")
	raw, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != total {
		t.Fatalf("expected %d ledger lines, got %d", total, len(lines))
	}
	ids := map[string]bool{}
	for i, line := range lines {
		var decoded CaptureRecord
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d not valid JSON: %v (%q)", i, err, line)
		}
		if decoded.ID == "" {
			t.Fatalf("line %d missing id", i)
		}
		if ids[decoded.ID] {
			t.Fatalf("duplicate capture id %s", decoded.ID)
		}
		ids[decoded.ID] = true
	}
}

// TestCaptureE2E_LedgerJSONLFormat verifies that an auto-written ledger
// record round-trips correctly: cap_ prefix, 64-char hex SHA, UTC
// timestamp, correct byte count, and matching surface/scenario/session
// fields.
func TestCaptureE2E_LedgerJSONLFormat(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-format")

	desktop := e2eWriteScreenshot(t, run.RepoRoot, "desktop.png", "format-desktop")
	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	filePreNote(t, store, run, "happy-path", "fmt-sess")
	before := time.Now().UTC().Add(-time.Second)
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "fmt-sess",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": desktop},
		PassAssertions: []string{"console_errors = 0"},
	}); err != nil {
		t.Fatalf("record browser: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger record, got %d", len(records))
	}
	loaded := records[0]

	if !strings.HasPrefix(loaded.ID, "cap_") {
		t.Fatalf("expected cap_ prefix, got %q", loaded.ID)
	}
	if len(loaded.ArtifactSHA256) != 64 {
		t.Fatalf("expected 64-char sha, got %d: %q", len(loaded.ArtifactSHA256), loaded.ArtifactSHA256)
	}
	hexRe := regexp.MustCompile("^[0-9a-f]{64}$")
	if !hexRe.MatchString(loaded.ArtifactSHA256) {
		t.Fatalf("sha not lower-hex: %q", loaded.ArtifactSHA256)
	}
	if _, err := hex.DecodeString(loaded.ArtifactSHA256); err != nil {
		t.Fatalf("sha does not hex-decode: %v", err)
	}

	// Confirm sha matches the submitted file.
	data, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	expectSHA := hex.EncodeToString(sum[:])
	if loaded.ArtifactSHA256 != expectSHA {
		t.Fatalf("sha mismatch: ledger %q vs file %q", loaded.ArtifactSHA256, expectSHA)
	}

	if loaded.CapturedAt.IsZero() {
		t.Fatal("timestamp is zero")
	}
	if loaded.CapturedAt.Location() != time.UTC {
		t.Fatalf("expected UTC timestamp, got %s", loaded.CapturedAt.Location())
	}
	if loaded.CapturedAt.Before(before) || loaded.CapturedAt.After(after) {
		t.Fatalf("timestamp %v outside window [%v, %v]", loaded.CapturedAt, before, after)
	}

	if loaded.Surface != SurfaceBrowser {
		t.Errorf("surface mismatch: %q", loaded.Surface)
	}
	if loaded.ScenarioID != "happy-path" {
		t.Errorf("scenario mismatch: %q", loaded.ScenarioID)
	}
	if loaded.SessionID != "fmt-sess" {
		t.Errorf("session mismatch: %q", loaded.SessionID)
	}
	if loaded.Label != "desktop" {
		t.Errorf("label mismatch: %q", loaded.Label)
	}

	info, err := os.Stat(loaded.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ArtifactBytes != info.Size() {
		t.Fatalf("byte count mismatch: ledger %d vs stat %d", loaded.ArtifactBytes, info.Size())
	}
}

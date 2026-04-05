package proctor

import (
	"context"
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

// fakeE2EBackend is an in-process capture backend used by the end-to-end
// adversarial tests. Every Capture() call writes the configured PNG bytes
// to the destination artifact path, optionally varies the content by
// scenario so SHA values differ, and returns a deterministic target map.
type fakeE2EBackend struct {
	base         []byte
	verification CaptureVerificationMode
	// perScenario, when non-empty, overrides the base bytes per scenario ID.
	perScenario map[string][]byte
}

func (b *fakeE2EBackend) Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error) {
	content := b.base
	if b.perScenario != nil {
		if scoped, ok := b.perScenario[opts.ScenarioID]; ok {
			content = scoped
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest.ArtifactPath), 0o755); err != nil {
		return CaptureResult{}, err
	}
	if err := os.WriteFile(dest.ArtifactPath, content, 0o644); err != nil {
		return CaptureResult{}, err
	}
	verification := b.verification
	if verification == "" {
		verification = CaptureVerifyNone
	}
	target := map[string]string{"scenario": opts.ScenarioID, "session": opts.SessionID}
	for k, v := range opts.Target {
		target[k] = v
	}
	return CaptureResult{Target: target, Verification: verification}, nil
}

// registerFakeE2EBackend swaps in a fake backend for the given surface and
// returns a cleanup func that restores the previous backend (or deletes the
// entry if one did not exist).
func registerFakeE2EBackend(t *testing.T, surface string, backend CaptureBackend) {
	t.Helper()
	prev, hadPrev := getCaptureBackend(surface)
	RegisterCaptureBackend(surface, backend)
	t.Cleanup(func() {
		captureBackendsMu.Lock()
		defer captureBackendsMu.Unlock()
		if hadPrev {
			captureBackends[surface] = prev
		} else {
			delete(captureBackends, surface)
		}
	})
}

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

// TestCaptureE2E_HappyPathRoundtrip verifies a capture → record chain
// writes a ledger entry, copies the captured PNG into scenario evidence,
// and records evidence successfully when the caller passes --capture-id.
func TestCaptureE2E_HappyPathRoundtrip(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-happy")
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("happy"),
		verification: CaptureVerifyMeta,
	})

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "browser-e2e-1",
		Label:      "desktop",
		Target:     map[string]string{"url": "http://127.0.0.1:3000/login"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !strings.HasPrefix(rec.ID, "cap_") {
		t.Fatalf("expected capture id prefix, got %q", rec.ID)
	}
	if _, err := os.Stat(rec.ArtifactPath); err != nil {
		t.Fatalf("capture artifact missing: %v", err)
	}

	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-e2e-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": rec.ArtifactPath},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
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
	if evidence[0].ScenarioID != "happy-path" || evidence[0].Surface != SurfaceBrowser {
		t.Fatalf("unexpected evidence: %#v", evidence[0])
	}

	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(records))
	}
	if records[0].ID != rec.ID {
		t.Fatalf("ledger id mismatch: %q vs %q", records[0].ID, rec.ID)
	}
	if records[0].ArtifactSHA256 != rec.ArtifactSHA256 {
		t.Fatalf("ledger sha mismatch")
	}
}

// TestCaptureE2E_TamperedArtifactRejected verifies that overwriting the
// captured PNG after capture causes RecordBrowser --capture-id to reject
// the submission (SHA mismatch is detected via "no submitted artifact
// matches capture").
func TestCaptureE2E_TamperedArtifactRejected(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-tamper")
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("tamper"),
		verification: CaptureVerifyMeta,
	})

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "tamper-1",
		Label:      "desktop",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Overwrite the artifact so its SHA no longer matches the ledger.
	tampered := make([]byte, int(DefaultMinScreenshotSize)+1)
	copy(tampered, []byte("tampered-bytes-"))
	for i := len("tampered-bytes-"); i < len(tampered); i++ {
		tampered[i] = byte((i * 7) & 0xff)
	}
	if err := os.WriteFile(rec.ArtifactPath, tampered, 0o644); err != nil {
		t.Fatal(err)
	}

	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "tamper-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": rec.ArtifactPath},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected tamper detection to reject record")
	}
	if !strings.Contains(err.Error(), "no submitted artifact matches capture") {
		t.Fatalf("expected SHA mismatch error, got %v", err)
	}

	// Ledger still holds a single entry (the original capture), evidence did
	// NOT get appended.
	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected ledger untouched, got %d entries", len(records))
	}
	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence after tampered record, got %d", len(evidence))
	}
}

// TestCaptureE2E_SessionMismatchRejected verifies that a capture-id
// captured under session=A is refused when the record command submits
// session=B.
func TestCaptureE2E_SessionMismatchRejected(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-session")
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("session"),
		verification: CaptureVerifyMeta,
	})

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "session-A",
		Label:      "desktop",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")
	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "session-B",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": rec.ArtifactPath},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected session mismatch rejection")
	}
	if !strings.Contains(err.Error(), "session") || !strings.Contains(err.Error(), "cannot bind") {
		t.Fatalf("expected session-binding error, got %v", err)
	}
}

// TestCaptureE2E_ScenarioMismatchRejected verifies that a capture-id
// captured for scenario S1 is refused when the record command references
// scenario S2.
func TestCaptureE2E_ScenarioMismatchRejected(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-scenario")
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("scenario"),
		verification: CaptureVerifyMeta,
	})

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "sess-x",
		Label:      "desktop",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")
	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "failure-path",
		SessionID:      "sess-x",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": rec.ArtifactPath},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected scenario mismatch rejection")
	}
	if !strings.Contains(err.Error(), "scenario") || !strings.Contains(err.Error(), "cannot bind") {
		t.Fatalf("expected scenario-binding error, got %v", err)
	}
}

// TestCaptureE2E_SurfaceMismatchRejected verifies that a capture recorded
// for surface=browser is refused when submitted to RecordIOS.
func TestCaptureE2E_SurfaceMismatchRejected(t *testing.T) {
	// Create an iOS-platform run so RecordIOS is permitted, and a browser
	// surface that we spoof to emit a capture whose run also contains an
	// iOS scenario (we re-use the same run).
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-e2e-surface")
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleIOSStartOptions())
	if err != nil {
		t.Fatal(err)
	}
	// Force-register a browser backend on an iOS run. Capture() itself does
	// not enforce platform ↔ surface, so we can still mint a "browser"
	// capture record and then try to bind it to iOS evidence.
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("surface"),
		verification: CaptureVerifyMeta,
	})

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "ios-sess-1",
		Label:      "desktop",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Build an iOS report + screenshot whose content matches the captured
	// PNG so we reach the surface-mismatch check rather than tripping the
	// "no submitted artifact matches capture" branch.
	iosReportBody := sampleIOSReport("com.example.pagena", "Library", "foreground", "iPhone 16 Pro", "iOS 18.2", 0, 0, 0)
	iosReport := filepath.Join(repo, "ios-report.json")
	if err := os.WriteFile(iosReport, []byte(iosReportBody), 0o644); err != nil {
		t.Fatal(err)
	}

	err = RecordIOS(store, run, IOSRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "ios-sess-1",
		ReportPath: iosReport,
		Screenshots: map[string]string{
			"library-screen": rec.ArtifactPath,
		},
		PassAssertions: []string{"screen contains Library", "bundle_id = com.example.pagena"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected surface mismatch rejection")
	}
	if !strings.Contains(err.Error(), "surface") || !strings.Contains(err.Error(), "cannot bind") {
		t.Fatalf("expected surface-binding error, got %v", err)
	}
}

// TestCaptureE2E_UnknownCaptureIDRejected verifies that an entirely
// fictitious capture id is rejected with a "not found in ledger" error.
func TestCaptureE2E_UnknownCaptureIDRejected(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-unknown")

	screenshot := writeScreenshotFixture(t, run.RepoRoot, "desktop.png", "desktop-real-image")
	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "unknown-sess",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": screenshot},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      "cap_NOTREAL",
	})
	if err == nil {
		t.Fatal("expected unknown capture id to be rejected")
	}
	if !strings.Contains(err.Error(), "not found in ledger") {
		t.Fatalf("expected 'not found in ledger' error, got %v", err)
	}
}

// TestCaptureE2E_DoubleRecordRejected verifies that the same capture
// artifact cannot be reused to satisfy two different scenarios. The
// duplicate-screenshot detector catches this before the capture binding
// check does, because the first record already copied the artifact into
// evidence.jsonl for scenario S1.
func TestCaptureE2E_DoubleRecordRejected(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-double")
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("double"),
		verification: CaptureVerifyMeta,
	})

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "double-sess",
		Label:      "desktop",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	// First record succeeds and binds the artifact to scenario happy-path.
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "double-sess",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": rec.ArtifactPath},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	}); err != nil {
		t.Fatalf("first record: %v", err)
	}

	// Second record attempts to reuse the same captured artifact for
	// scenario failure-path. The duplicate-screenshot check fires first,
	// because the artifact with this SHA already landed in scenario happy
	// path's evidence. Either failure mode is acceptable; both demonstrate
	// that a single capture cannot be recycled across scenarios.
	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "failure-path",
		SessionID:      "double-sess",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": rec.ArtifactPath},
		PassAssertions: []string{"console_errors = 0"},
		CaptureID:      rec.ID,
	})
	if err == nil {
		t.Fatal("expected double-record rejection")
	}
	msg := err.Error()
	okScenarioBind := strings.Contains(msg, "scenario") && strings.Contains(msg, "cannot bind")
	okDupe := strings.Contains(msg, "each scenario requires unique evidence")
	if !okScenarioBind && !okDupe {
		t.Fatalf("expected scenario-binding OR duplicate-screenshot error, got %v", err)
	}
}

// TestCaptureE2E_BackCompatNoCaptureID verifies that omitting --capture-id
// continues to work as before (an unbound record succeeds when all other
// validation passes).
func TestCaptureE2E_BackCompatNoCaptureID(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-backcompat")

	screenshot := writeScreenshotFixture(t, run.RepoRoot, "desktop.png", "backcompat-unique-image")
	report := e2eBrowserReport(t, run.RepoRoot, "report.json", "http://127.0.0.1:3000/dashboard")

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "backcompat-sess",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": screenshot},
		PassAssertions: []string{"console_errors = 0"},
		// CaptureID intentionally left empty.
	}); err != nil {
		t.Fatalf("back-compat record: %v", err)
	}

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(evidence))
	}

	// Ledger is empty because no capture was ever run.
	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("expected empty ledger, got %d entries", len(records))
	}
}

// TestCaptureE2E_ConcurrentCaptures spawns N goroutines performing
// Capture() calls in parallel. All captures should succeed, all IDs should
// be distinct, the ledger file should end up well-formed (JSONL, one
// record per line), and the number of lines should match the number of
// workers. Intended to be exercised with -race.
func TestCaptureE2E_ConcurrentCaptures(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-concurrent")

	// Give each goroutine its own per-scenario bytes so Capture() can
	// freely write unique artifacts.
	perScenario := map[string][]byte{
		"happy-path":   e2eScreenshotBytes("conc-happy"),
		"failure-path": e2eScreenshotBytes("conc-failure"),
	}
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("conc-default"),
		verification: CaptureVerifyMeta,
		perScenario:  perScenario,
	})

	const workers = 10
	scenarios := []string{"happy-path", "failure-path"}
	results := make([]CaptureRecord, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			scen := scenarios[i%len(scenarios)]
			rec, err := Capture(context.Background(), store, run, CaptureOptions{
				Surface:    SurfaceBrowser,
				ScenarioID: scen,
				SessionID:  fmt.Sprintf("conc-sess-%d", i),
				Label:      "desktop",
			})
			results[i] = rec
			errs[i] = err
		}()
	}
	wg.Wait()

	ids := map[string]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if ids[results[i].ID] {
			t.Fatalf("duplicate capture id %s", results[i].ID)
		}
		ids[results[i].ID] = true
	}

	// Read ledger file raw and confirm one well-formed JSON object per
	// line, matching the number of workers.
	ledgerPath := filepath.Join(store.RunDir(run), "captures.jsonl")
	raw, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != workers {
		t.Fatalf("expected %d ledger lines, got %d", workers, len(lines))
	}
	for i, line := range lines {
		var decoded CaptureRecord
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d not valid JSON: %v (%q)", i, err, line)
		}
		if decoded.ID == "" {
			t.Fatalf("line %d missing id", i)
		}
	}

	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != workers {
		t.Fatalf("expected %d ledger records, got %d", workers, len(records))
	}
}

// TestCaptureE2E_LedgerJSONLFormat verifies that loaded capture records
// round-trip correctly: timestamps are UTC, SHA is hex-encoded 64-char,
// target is populated, and IDs match the expected cap_ format.
func TestCaptureE2E_LedgerJSONLFormat(t *testing.T) {
	store, run := webRun(t, "https://github.com/nclandrei/proctor-e2e-format")
	registerFakeE2EBackend(t, SurfaceBrowser, &fakeE2EBackend{
		base:         e2eScreenshotBytes("format"),
		verification: CaptureVerifyMeta,
	})

	before := time.Now().UTC().Add(-time.Second)
	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "fmt-sess",
		Label:      "desktop",
		Target:     map[string]string{"url": "http://example.com"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	loaded := records[0]

	// ID format.
	if !strings.HasPrefix(loaded.ID, "cap_") {
		t.Fatalf("expected cap_ prefix, got %q", loaded.ID)
	}

	// SHA format.
	if len(loaded.ArtifactSHA256) != 64 {
		t.Fatalf("expected 64-char hex sha, got %d chars: %q", len(loaded.ArtifactSHA256), loaded.ArtifactSHA256)
	}
	hexRe := regexp.MustCompile("^[0-9a-f]{64}$")
	if !hexRe.MatchString(loaded.ArtifactSHA256) {
		t.Fatalf("sha is not lower-case hex: %q", loaded.ArtifactSHA256)
	}
	if _, err := hex.DecodeString(loaded.ArtifactSHA256); err != nil {
		t.Fatalf("sha does not hex-decode: %v", err)
	}

	// Timestamp must be UTC, non-zero, and within the captured window.
	if loaded.CapturedAt.IsZero() {
		t.Fatal("timestamp is zero")
	}
	if loaded.CapturedAt.Location() != time.UTC {
		t.Fatalf("expected UTC timestamp, got %s", loaded.CapturedAt.Location())
	}
	if loaded.CapturedAt.Before(before) || loaded.CapturedAt.After(after) {
		t.Fatalf("timestamp %v outside window [%v, %v]", loaded.CapturedAt, before, after)
	}

	// Target was populated by the fake backend.
	if loaded.Target["scenario"] != "happy-path" {
		t.Fatalf("expected scenario target, got %#v", loaded.Target)
	}
	if loaded.Target["url"] != "http://example.com" {
		t.Fatalf("expected passthrough url target, got %#v", loaded.Target)
	}
	if loaded.ID != rec.ID {
		t.Fatalf("loaded id %q vs returned id %q", loaded.ID, rec.ID)
	}

	// Byte count matches the file on disk.
	info, err := os.Stat(loaded.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ArtifactBytes != info.Size() {
		t.Fatalf("byte count mismatch: ledger %d vs stat %d", loaded.ArtifactBytes, info.Size())
	}
}

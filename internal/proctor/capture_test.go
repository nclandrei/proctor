package proctor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCaptureBackend writes a deterministic PNG to the destination so tests can
// exercise the Capture engine without touching real browsers, simulators, or
// desktop apps.
type fakeCaptureBackend struct {
	content      []byte
	transcript   []byte
	target       map[string]string
	verification CaptureVerificationMode
	writeErr     error
}

func (b *fakeCaptureBackend) Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error) {
	if b.writeErr != nil {
		return CaptureResult{}, b.writeErr
	}
	if err := os.WriteFile(dest.ArtifactPath, b.content, 0o644); err != nil {
		return CaptureResult{}, err
	}
	transcriptWritten := false
	if b.transcript != nil && dest.TranscriptPath != "" {
		if err := os.WriteFile(dest.TranscriptPath, b.transcript, 0o644); err != nil {
			return CaptureResult{}, err
		}
		transcriptWritten = true
	}
	verification := b.verification
	if verification == "" {
		verification = CaptureVerifyNone
	}
	return CaptureResult{
		Target:            b.target,
		Verification:      verification,
		TranscriptWritten: transcriptWritten,
	}, nil
}

func registerFakeCaptureBackend(t *testing.T, surface string, backend CaptureBackend) {
	t.Helper()
	prev, hadPrev := getCaptureBackend(surface)
	RegisterCaptureBackend(surface, backend)
	t.Cleanup(func() {
		if hadPrev {
			RegisterCaptureBackend(surface, prev)
		}
	})
}

func getCaptureBackend(surface string) (CaptureBackend, bool) {
	captureBackendsMu.RLock()
	defer captureBackendsMu.RUnlock()
	b, ok := captureBackends[surface]
	return b, ok
}

func captureFixtureBytes() []byte {
	minSize := int(DefaultMinScreenshotSize) + 1
	buf := make([]byte, minSize)
	copy(buf, []byte("png-data-"))
	for i := len("png-data-"); i < minSize; i++ {
		buf[i] = byte(i & 0xff)
	}
	return buf
}

func sampleRun(t *testing.T) (*Store, Run, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}
	return store, run, repo
}

func TestCaptureIDFormat(t *testing.T) {
	id, err := newCaptureID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "cap_") {
		t.Fatalf("expected cap_ prefix, got %s", id)
	}
	token := strings.TrimPrefix(id, "cap_")
	if len(token) != captureTokenLength {
		t.Fatalf("expected token length %d, got %d (%s)", captureTokenLength, len(token), token)
	}
	match, err := regexp.MatchString("^[A-Z0-9]{6}$", token)
	if err != nil {
		t.Fatal(err)
	}
	if !match {
		t.Fatalf("token must be uppercase alphanumeric, got %s", token)
	}
}

func TestCaptureNonceFormat(t *testing.T) {
	nonce, err := newCaptureNonce()
	if err != nil {
		t.Fatal(err)
	}
	if len(nonce) != captureTokenLength {
		t.Fatalf("expected nonce length %d, got %d", captureTokenLength, len(nonce))
	}
	match, err := regexp.MatchString("^[A-Z0-9]{6}$", nonce)
	if err != nil {
		t.Fatal(err)
	}
	if !match {
		t.Fatalf("nonce must be uppercase alphanumeric, got %s", nonce)
	}
}

func TestCaptureIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 256; i++ {
		id, err := newCaptureID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate capture id: %s", id)
		}
		seen[id] = true
	}
}

func TestCaptureLedgerRoundTrip(t *testing.T) {
	store, run, _ := sampleRun(t)
	ledger := store.CaptureLedger(run)

	records := []CaptureRecord{
		{
			ID: "cap_AAAAAA", RunID: run.ID, ScenarioID: "happy-path", SessionID: "s-1",
			Surface: SurfaceBrowser, Label: "desktop", Nonce: "NONCE1",
			ArtifactPath: "/tmp/a.png", ArtifactSHA256: "aaaa", ArtifactBytes: 123,
			Target: map[string]string{"url": "http://example.com"}, Verification: CaptureVerifyMeta,
			CapturedAt: time.Now().UTC(),
		},
		{
			ID: "cap_BBBBBB", RunID: run.ID, ScenarioID: "failure-path", SessionID: "s-1",
			Surface: SurfaceBrowser, Label: "mobile", Nonce: "NONCE2",
			ArtifactPath: "/tmp/b.png", ArtifactSHA256: "bbbb", ArtifactBytes: 456,
			Target: map[string]string{"url": "http://example.com/fail"}, Verification: CaptureVerifyPixel,
			CapturedAt: time.Now().UTC(),
		},
	}
	for _, rec := range records {
		if err := ledger.Append(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	loaded, err := ledger.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != len(records) {
		t.Fatalf("expected %d records, got %d", len(records), len(loaded))
	}
	for i, rec := range records {
		if loaded[i].ID != rec.ID {
			t.Fatalf("record %d: expected id %s, got %s", i, rec.ID, loaded[i].ID)
		}
		if loaded[i].Verification != rec.Verification {
			t.Fatalf("record %d: expected verification %s, got %s", i, rec.Verification, loaded[i].Verification)
		}
	}

	got, ok, err := ledger.FindByID("cap_BBBBBB")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !ok {
		t.Fatal("expected to find cap_BBBBBB")
	}
	if got.Label != "mobile" {
		t.Fatalf("expected label mobile, got %s", got.Label)
	}

	_, ok, err = ledger.FindByID("cap_MISSING")
	if err != nil {
		t.Fatalf("find missing: %v", err)
	}
	if ok {
		t.Fatal("expected missing capture id to report ok=false")
	}
}

func TestCaptureLedgerConcurrentAppend(t *testing.T) {
	store, run, _ := sampleRun(t)
	ledger := store.CaptureLedger(run)

	const workers = 8
	const perWorker = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		w := w
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				rec := CaptureRecord{
					ID:             fmt.Sprintf("cap_%02d%04d", w, i),
					RunID:          run.ID,
					ScenarioID:     "happy-path",
					SessionID:      "s-1",
					Surface:        SurfaceBrowser,
					Label:          "desktop",
					Nonce:          "NONCE0",
					ArtifactPath:   "/tmp/x.png",
					ArtifactSHA256: "aabb",
					ArtifactBytes:  42,
					CapturedAt:     time.Now().UTC(),
				}
				if err := ledger.Append(rec); err != nil {
					t.Errorf("append: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	loaded, err := ledger.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != workers*perWorker {
		t.Fatalf("expected %d records, got %d", workers*perWorker, len(loaded))
	}
}

func TestCaptureRejectsUnknownSurface(t *testing.T) {
	store, run, _ := sampleRun(t)
	_, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    "nope",
		ScenarioID: "happy-path",
		SessionID:  "s-1",
		Label:      "desktop",
	})
	if err == nil {
		t.Fatal("expected error for unknown surface")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected error to mention surface, got %v", err)
	}
}

func TestCaptureRejectsMissingScenario(t *testing.T) {
	store, run, _ := sampleRun(t)
	_, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "not-a-scenario",
		SessionID:  "s-1",
		Label:      "desktop",
	})
	if err == nil {
		t.Fatal("expected error for missing scenario")
	}
	if !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("expected unknown scenario error, got %v", err)
	}
}

func TestCaptureRejectsMissingSession(t *testing.T) {
	store, run, _ := sampleRun(t)
	_, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
	})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "session") {
		t.Fatalf("expected session error, got %v", err)
	}
}

func TestCaptureHappyPath(t *testing.T) {
	store, run, _ := sampleRun(t)
	pngBytes := captureFixtureBytes()
	backend := &fakeCaptureBackend{
		content:      pngBytes,
		target:       map[string]string{"url": "http://127.0.0.1:3000/login"},
		verification: CaptureVerifyMeta,
	}
	registerFakeCaptureBackend(t, SurfaceBrowser, backend)

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		Label:      "desktop",
		Target:     map[string]string{"url": "http://127.0.0.1:3000/login"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !strings.HasPrefix(rec.ID, "cap_") {
		t.Fatalf("expected capture id prefix cap_, got %s", rec.ID)
	}
	if rec.RunID != run.ID {
		t.Fatalf("expected run id %s, got %s", run.ID, rec.RunID)
	}
	if rec.Surface != SurfaceBrowser {
		t.Fatalf("expected surface browser, got %s", rec.Surface)
	}
	if rec.ScenarioID != "happy-path" {
		t.Fatalf("expected scenario happy-path, got %s", rec.ScenarioID)
	}
	if rec.SessionID != "browser-1" {
		t.Fatalf("expected session browser-1, got %s", rec.SessionID)
	}
	if rec.ArtifactBytes != int64(len(pngBytes)) {
		t.Fatalf("expected bytes %d, got %d", len(pngBytes), rec.ArtifactBytes)
	}
	if rec.Verification != CaptureVerifyMeta {
		t.Fatalf("expected verification meta, got %s", rec.Verification)
	}
	if rec.Target["url"] != "http://127.0.0.1:3000/login" {
		t.Fatalf("expected target url to be passed through, got %v", rec.Target)
	}
	if rec.Nonce == "" || len(rec.Nonce) != captureTokenLength {
		t.Fatalf("expected 6-char nonce, got %q", rec.Nonce)
	}

	// Verify on-disk content and hash.
	data, err := os.ReadFile(rec.ArtifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != rec.ArtifactSHA256 {
		t.Fatalf("sha mismatch: ledger has %s, file hashes to %s", rec.ArtifactSHA256, got)
	}

	// Verify ledger persistence.
	records, err := store.CaptureLedger(run).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(records))
	}
	if records[0].ID != rec.ID {
		t.Fatalf("expected ledger id %s, got %s", rec.ID, records[0].ID)
	}
}

func TestCaptureRejectsTooSmallArtifact(t *testing.T) {
	store, run, _ := sampleRun(t)
	backend := &fakeCaptureBackend{
		content: []byte("tiny"),
	}
	registerFakeCaptureBackend(t, SurfaceBrowser, backend)

	_, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		Label:      "desktop",
	})
	if err == nil {
		t.Fatal("expected error for too-small capture artifact")
	}
	if !strings.Contains(err.Error(), "too small") {
		t.Fatalf("expected error to mention too small, got %v", err)
	}
}

func TestCaptureBackendErrorPropagates(t *testing.T) {
	store, run, _ := sampleRun(t)
	backend := &fakeCaptureBackend{
		writeErr: fmt.Errorf("backend exploded"),
	}
	registerFakeCaptureBackend(t, SurfaceBrowser, backend)

	_, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		Label:      "desktop",
	})
	if err == nil {
		t.Fatal("expected backend error to propagate")
	}
	if !strings.Contains(err.Error(), "backend exploded") {
		t.Fatalf("expected backend error text, got %v", err)
	}
}

func TestCaptureDefaultLabel(t *testing.T) {
	store, run, _ := sampleRun(t)
	backend := &fakeCaptureBackend{content: captureFixtureBytes()}
	registerFakeCaptureBackend(t, SurfaceBrowser, backend)

	rec, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if rec.Label != "main" {
		t.Fatalf("expected default label main, got %q", rec.Label)
	}
}

func TestStubBackendReturnsNotImplemented(t *testing.T) {
	// The stubs registered at init() should produce a clear error until a real
	// backend overwrites them. We snapshot, replace with a fresh stub, test, then
	// restore.
	prev, hadPrev := getCaptureBackend(SurfaceBrowser)
	RegisterCaptureBackend(SurfaceBrowser, captureStubBackend{surface: SurfaceBrowser})
	t.Cleanup(func() {
		if hadPrev {
			RegisterCaptureBackend(SurfaceBrowser, prev)
		}
	})

	store, run, _ := sampleRun(t)
	_, err := Capture(context.Background(), store, run, CaptureOptions{
		Surface:    SurfaceBrowser,
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		Label:      "desktop",
	})
	if err == nil {
		t.Fatal("expected stub backend to return not implemented")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("expected not implemented error, got %v", err)
	}
}

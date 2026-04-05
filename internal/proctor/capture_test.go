package proctor

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

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
	id, err := GenerateCaptureID()
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

func TestCaptureIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 256; i++ {
		id, err := GenerateCaptureID()
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
			Surface: SurfaceBrowser, Label: "desktop",
			ArtifactPath: "/tmp/a.png", ArtifactSHA256: "aaaa", ArtifactBytes: 123,
			Target: map[string]string{"url": "http://example.com"}, Verification: CaptureVerifyMeta,
			CapturedAt: time.Now().UTC(),
		},
		{
			ID: "cap_BBBBBB", RunID: run.ID, ScenarioID: "failure-path", SessionID: "s-1",
			Surface: SurfaceBrowser, Label: "mobile",
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

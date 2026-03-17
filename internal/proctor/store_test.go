package proctor

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestArtifactPathsAreAppendOnlyPerRecording(t *testing.T) {
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

	t.Run("copy artifact", func(t *testing.T) {
		firstSource := writeFixture(t, repo, "desktop-first.png", "desktop-image-first")
		first, err := store.CopyArtifact(run, SurfaceBrowser, "happy-path", "desktop", firstSource)
		if err != nil {
			t.Fatal(err)
		}

		secondSource := writeFixture(t, repo, "desktop-second.png", "desktop-image-second")
		second, err := store.CopyArtifact(run, SurfaceBrowser, "happy-path", "desktop", secondSource)
		if err != nil {
			t.Fatal(err)
		}

		if first.Path == second.Path {
			t.Fatalf("expected repeated copied artifacts to get unique paths, got %q", first.Path)
		}
		if err := store.VerifyArtifactHash(run, first); err != nil {
			t.Fatalf("expected first copied artifact to remain valid after second recording, got %v", err)
		}
		if err := store.VerifyArtifactHash(run, second); err != nil {
			t.Fatalf("expected second copied artifact hash to verify, got %v", err)
		}
	})

	t.Run("write artifact", func(t *testing.T) {
		first, err := store.WriteArtifact(run, SurfaceCurl, "happy-path", "curl-transcript", ".txt", []byte("first transcript"))
		if err != nil {
			t.Fatal(err)
		}

		second, err := store.WriteArtifact(run, SurfaceCurl, "happy-path", "curl-transcript", ".txt", []byte("second transcript"))
		if err != nil {
			t.Fatal(err)
		}

		if first.Path == second.Path {
			t.Fatalf("expected repeated written artifacts to get unique paths, got %q", first.Path)
		}
		if err := store.VerifyArtifactHash(run, first); err != nil {
			t.Fatalf("expected first written artifact to remain valid after second recording, got %v", err)
		}
		if err := store.VerifyArtifactHash(run, second); err != nil {
			t.Fatalf("expected second written artifact hash to verify, got %v", err)
		}
	})
}

func TestConcurrentAppendEvidenceProducesValidJSONL(t *testing.T) {
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

	const workers = 8
	const perWorker = 10

	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				ev := Evidence{
					ID:         fmt.Sprintf("ev-w%d-i%d", workerID, i),
					RunID:      run.ID,
					ScenarioID: "happy-path",
					Surface:    SurfaceBrowser,
					Tier:       TierRegisteredRun,
					CreatedAt:  time.Now().UTC(),
					Title:      fmt.Sprintf("concurrent evidence %d-%d", workerID, i),
				}
				if err := store.AppendEvidence(run, ev); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent append failed: %v", err)
	}

	evidence, err := store.LoadEvidence(run)
	if err != nil {
		t.Fatalf("failed to load evidence after concurrent writes: %v", err)
	}
	if len(evidence) != workers*perWorker {
		t.Fatalf("expected %d evidence entries, got %d", workers*perWorker, len(evidence))
	}

	seen := map[string]bool{}
	for _, ev := range evidence {
		if seen[ev.ID] {
			t.Fatalf("duplicate evidence ID: %s", ev.ID)
		}
		seen[ev.ID] = true
	}
}

package proctor

import "testing"

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

package proctor

import (
	"strings"
	"sync"
	"testing"
)

func newNotesFixture(t *testing.T) (*Store, Run) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-notes-test")
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

func TestFilePreNoteHappyPath(t *testing.T) {
	store, run := newNotesFixture(t)

	note, err := FilePreNote(store, run, "happy-path", "browser-1", testPreNoteText)
	if err != nil {
		t.Fatalf("FilePreNote failed: %v", err)
	}
	if !strings.HasPrefix(note.ID, "note_") {
		t.Fatalf("expected note id to have note_ prefix, got %q", note.ID)
	}
	if note.Scenario != "happy-path" || note.Session != "browser-1" {
		t.Fatalf("expected scenario+session to round-trip, got %#v", note)
	}
	if note.Notes != testPreNoteText {
		t.Fatalf("expected notes to round-trip, got %q", note.Notes)
	}
	if note.RunID != run.ID {
		t.Fatalf("expected RunID to match, got %q want %q", note.RunID, run.ID)
	}
	if note.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}
}

func TestFilePreNoteRequiresMinLength(t *testing.T) {
	store, run := newNotesFixture(t)

	_, err := FilePreNote(store, run, "happy-path", "browser-1", "too short")
	if err == nil {
		t.Fatal("expected short pre-note to be rejected")
	}
	if !strings.Contains(err.Error(), "must describe") {
		t.Fatalf("expected pre-note length error, got: %v", err)
	}
}

func TestFilePreNoteRequiresAllFields(t *testing.T) {
	store, run := newNotesFixture(t)

	_, err := FilePreNote(store, run, "", "browser-1", testPreNoteText)
	if err == nil || !strings.Contains(err.Error(), "--scenario is required") {
		t.Fatalf("expected --scenario validation, got: %v", err)
	}
	_, err = FilePreNote(store, run, "happy-path", "", testPreNoteText)
	if err == nil || !strings.Contains(err.Error(), "--session is required") {
		t.Fatalf("expected --session validation, got: %v", err)
	}
	_, err = FilePreNote(store, run, "happy-path", "browser-1", "")
	if err == nil || !strings.Contains(err.Error(), "--notes is required") {
		t.Fatalf("expected --notes validation, got: %v", err)
	}
	_, err = FilePreNote(store, run, "happy-path", "browser-1", "   \t\n  ")
	if err == nil || !strings.Contains(err.Error(), "--notes is required") {
		t.Fatalf("expected whitespace-only --notes to be rejected, got: %v", err)
	}
}

func TestFilePreNoteRejectsUnknownScenario(t *testing.T) {
	store, run := newNotesFixture(t)

	_, err := FilePreNote(store, run, "nonexistent-scenario", "browser-1", testPreNoteText)
	if err == nil || !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("expected unknown scenario to be rejected, got: %v", err)
	}
}

func TestLoadPreNotesRoundTrip(t *testing.T) {
	store, run := newNotesFixture(t)

	first, err := FilePreNote(store, run, "happy-path", "browser-1", testPreNoteText)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FilePreNote(store, run, "failure-path", "browser-1", testPreNoteText+" second")
	if err != nil {
		t.Fatal(err)
	}

	notes, err := store.LoadPreNotes(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 pre-notes loaded, got %d", len(notes))
	}
	if notes[0].ID != first.ID || notes[1].ID != second.ID {
		t.Fatalf("expected notes to load in file order, got %v and %v", notes[0].ID, notes[1].ID)
	}
	if notes[0].Scenario != "happy-path" || notes[1].Scenario != "failure-path" {
		t.Fatalf("expected scenarios to round-trip, got %v", notes)
	}
}

func TestConcurrentPreNoteAppends(t *testing.T) {
	store, run := newNotesFixture(t)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			_, err := FilePreNote(store, run, "happy-path", "browser-concurrent", testPreNoteText)
			errs[i] = err
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	notes, err := store.LoadPreNotes(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != workers {
		t.Fatalf("expected %d concurrent pre-notes, got %d", workers, len(notes))
	}
	seen := map[string]bool{}
	for _, note := range notes {
		if seen[note.ID] {
			t.Fatalf("duplicate pre-note id %s", note.ID)
		}
		seen[note.ID] = true
	}
}

func TestHasPreNoteLookup(t *testing.T) {
	store, run := newNotesFixture(t)

	ok, err := store.HasPreNote(run, "happy-path", "browser-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no pre-note before file")
	}
	if _, err := FilePreNote(store, run, "happy-path", "browser-1", testPreNoteText); err != nil {
		t.Fatal(err)
	}
	ok, err = store.HasPreNote(run, "happy-path", "browser-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected pre-note after file")
	}
	// Different session should not satisfy the (scenario, session) gate.
	ok, err = store.HasPreNote(run, "happy-path", "browser-2")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected different session to not satisfy the gate")
	}
}

func TestMultiplePreNotesPerScenarioSessionAllowed(t *testing.T) {
	store, run := newNotesFixture(t)

	for i := 0; i < 3; i++ {
		if _, err := FilePreNote(store, run, "happy-path", "browser-1", testPreNoteText); err != nil {
			t.Fatalf("additional pre-note #%d: %v", i, err)
		}
	}
	notes, err := store.LoadPreNotes(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 3 {
		t.Fatalf("expected 3 pre-notes in audit trail, got %d", len(notes))
	}
}

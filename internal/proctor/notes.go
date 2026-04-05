package proctor

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// NotesLedger reads and writes notes.jsonl for a run. Pre-test notes are
// appended before any record call so the agent commits to what they intend
// to test at the moment of action.
type NotesLedger struct {
	store *Store
	run   Run
}

// NotesLedger returns a ledger handle for the given run.
func (s *Store) NotesLedger(run Run) *NotesLedger {
	return &NotesLedger{store: s, run: run}
}

// NotesPath returns the absolute path to notes.jsonl for the given run.
func (s *Store) NotesPath(run Run) string {
	return filepath.Join(s.RunDir(run), "notes.jsonl")
}

// AppendPreNote writes a pre-test note to notes.jsonl using an exclusive
// file lock. Multiple notes per (scenario, session) are allowed; subsequent
// notes form an audit trail of additional intent.
func (s *Store) AppendPreNote(run Run, note PreNote) error {
	return s.NotesLedger(run).Append(note)
}

// LoadPreNotes returns every pre-note in notes.jsonl in file order.
func (s *Store) LoadPreNotes(run Run) ([]PreNote, error) {
	return s.NotesLedger(run).Load()
}

// HasPreNote reports whether any pre-test note has been filed for the given
// scenario and session.
func (s *Store) HasPreNote(run Run, scenario, session string) (bool, error) {
	return s.NotesLedger(run).HasForScenarioSession(scenario, session)
}

func (l *NotesLedger) path() string {
	return filepath.Join(l.store.RunDir(l.run), "notes.jsonl")
}

// Append writes a pre-note record to the ledger using an exclusive file lock.
func (l *NotesLedger) Append(note PreNote) error {
	path := l.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock notes ledger: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	enc := json.NewEncoder(file)
	return enc.Encode(note)
}

// Load returns every pre-note record from the ledger using a shared read lock.
func (l *NotesLedger) Load() ([]PreNote, error) {
	path := l.path()
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return nil, fmt.Errorf("lock notes ledger: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	var notes []PreNote
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec PreNote
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, err
		}
		notes = append(notes, rec)
	}
	return notes, scanner.Err()
}

// HasForScenarioSession reports whether a pre-note exists for the given
// scenario+session combination.
func (l *NotesLedger) HasForScenarioSession(scenario, session string) (bool, error) {
	notes, err := l.Load()
	if err != nil {
		return false, err
	}
	for _, note := range notes {
		if note.Scenario == scenario && note.Session == session {
			return true, nil
		}
	}
	return false, nil
}

// HasForScenario reports whether any pre-note has been filed for the given
// scenario (any session). Used by the done gate: once a scenario has any
// pre-note, the forcing function has been satisfied.
func (l *NotesLedger) HasForScenario(scenario string) (bool, error) {
	notes, err := l.Load()
	if err != nil {
		return false, err
	}
	for _, note := range notes {
		if note.Scenario == scenario {
			return true, nil
		}
	}
	return false, nil
}

// preNoteTokenLength is the fixed length for pre-note ID tokens.
const preNoteTokenLength = 6

// GeneratePreNoteID returns a fresh pre-note ID of the form "note_XK7Q2M".
// It shares the capture ID alphabet so the two identifiers look consistent
// in the ledger files.
func GeneratePreNoteID() (string, error) {
	token, err := randomPreNoteToken()
	if err != nil {
		return "", err
	}
	return "note_" + token, nil
}

func randomPreNoteToken() (string, error) {
	buf := make([]byte, preNoteTokenLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, preNoteTokenLength)
	for i, b := range buf {
		out[i] = captureTokenAlphabet[int(b)%len(captureTokenAlphabet)]
	}
	return string(out), nil
}

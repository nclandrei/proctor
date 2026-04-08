package proctor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ScreenshotLogLedger reads and writes screenshot-log.jsonl for a run.
// Each entry records one step the agent took during verification: what
// it did, what it saw in the screenshot, and how that compares to the
// scenario requirements. The ledger is append-only.
type ScreenshotLogLedger struct {
	store *Store
	run   Run
}

// ScreenshotLogLedger returns a ledger handle for the given run.
func (s *Store) ScreenshotLogLedger(run Run) *ScreenshotLogLedger {
	return &ScreenshotLogLedger{store: s, run: run}
}

// ScreenshotLogPath returns the absolute path to screenshot-log.jsonl.
func (s *Store) ScreenshotLogPath(run Run) string {
	return filepath.Join(s.RunDir(run), "screenshot-log.jsonl")
}

func (l *ScreenshotLogLedger) path() string {
	return filepath.Join(l.store.RunDir(l.run), "screenshot-log.jsonl")
}

// Append writes a screenshot log entry using an exclusive file lock.
func (l *ScreenshotLogLedger) Append(entry ScreenshotLogEntry) error {
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
		return fmt.Errorf("lock screenshot log: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	enc := json.NewEncoder(file)
	return enc.Encode(entry)
}

// Load returns every screenshot log entry from the ledger in file order.
func (l *ScreenshotLogLedger) Load() ([]ScreenshotLogEntry, error) {
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
		return nil, fmt.Errorf("lock screenshot log: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	var entries []ScreenshotLogEntry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry ScreenshotLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// LoadForScenario returns screenshot log entries for a specific scenario.
func (l *ScreenshotLogLedger) LoadForScenario(scenarioID string) ([]ScreenshotLogEntry, error) {
	all, err := l.Load()
	if err != nil {
		return nil, err
	}
	var filtered []ScreenshotLogEntry
	for _, entry := range all {
		if entry.ScenarioID == scenarioID {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

// NextStep returns the next step number for a (scenario, session) pair.
func (l *ScreenshotLogLedger) NextStep(scenarioID, sessionID string) (int, error) {
	all, err := l.Load()
	if err != nil {
		return 0, err
	}
	maxStep := 0
	for _, entry := range all {
		if entry.ScenarioID == scenarioID && entry.SessionID == sessionID {
			if entry.Step > maxStep {
				maxStep = entry.Step
			}
		}
	}
	return maxStep + 1, nil
}

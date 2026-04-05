package proctor

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// CaptureVerificationMode indicates how proctor verified the capture targets the right thing.
type CaptureVerificationMode string

const (
	// CaptureVerifyNone means only ledger/SHA binding is enforced. No target metadata
	// was cross-checked and no nonce pixel template match was performed.
	CaptureVerifyNone CaptureVerificationMode = "none"
	// CaptureVerifyMeta means the target metadata (URL, window id, bundle id, etc.)
	// was cross-checked against the capture target.
	CaptureVerifyMeta CaptureVerificationMode = "meta"
	// CaptureVerifyPixel means the nonce was rendered and template-matched in the
	// captured image.
	CaptureVerifyPixel CaptureVerificationMode = "pixel"
)

// ArtifactCapture is an artifact kind marker for proctor-managed captures.
const ArtifactCapture = "capture"

// CaptureRecord is one entry in the run's capture ledger (captures.jsonl).
type CaptureRecord struct {
	ID               string                  `json:"id"`
	RunID            string                  `json:"run_id"`
	ScenarioID       string                  `json:"scenario_id"`
	SessionID        string                  `json:"session_id"`
	Surface          string                  `json:"surface"`
	Label            string                  `json:"label"`
	ArtifactPath     string                  `json:"artifact_path"`
	ArtifactSHA256   string                  `json:"artifact_sha256"`
	ArtifactBytes    int64                   `json:"artifact_bytes"`
	TranscriptPath   string                  `json:"transcript_path,omitempty"`
	TranscriptSHA256 string                  `json:"transcript_sha256,omitempty"`
	Target           map[string]string       `json:"target"`
	Verification     CaptureVerificationMode `json:"verification"`
	CapturedAt       time.Time               `json:"captured_at"`
}

// CaptureLedger reads and writes captures.jsonl for a run.
type CaptureLedger struct {
	store *Store
	run   Run
}

// CaptureLedger returns a ledger handle for the given run.
func (s *Store) CaptureLedger(run Run) *CaptureLedger {
	return &CaptureLedger{store: s, run: run}
}

func (l *CaptureLedger) path() string {
	return filepath.Join(l.store.RunDir(l.run), "captures.jsonl")
}

// Append writes a capture record to the ledger using an exclusive file lock.
func (l *CaptureLedger) Append(rec CaptureRecord) error {
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
		return fmt.Errorf("lock capture ledger: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	enc := json.NewEncoder(file)
	return enc.Encode(rec)
}

// Load returns every capture record from the ledger using a shared read lock.
func (l *CaptureLedger) Load() ([]CaptureRecord, error) {
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
		return nil, fmt.Errorf("lock capture ledger: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	var records []CaptureRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec CaptureRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

// FindByID returns the ledger record matching the given capture ID.
func (l *CaptureLedger) FindByID(id string) (CaptureRecord, bool, error) {
	records, err := l.Load()
	if err != nil {
		return CaptureRecord{}, false, err
	}
	for _, rec := range records {
		if rec.ID == id {
			return rec, true, nil
		}
	}
	return CaptureRecord{}, false, nil
}

// captureTokenAlphabet is the fixed character set used for capture IDs.
// It is upper-case alphanumeric to stay readable.
const captureTokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// captureTokenLength is the fixed length for capture ID tokens.
const captureTokenLength = 6

// GenerateCaptureID returns a fresh capture ID of the form "cap_XK7Q2M".
func GenerateCaptureID() (string, error) {
	token, err := randomCaptureToken()
	if err != nil {
		return "", err
	}
	return "cap_" + token, nil
}

func randomCaptureToken() (string, error) {
	buf := make([]byte, captureTokenLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, captureTokenLength)
	for i, b := range buf {
		out[i] = captureTokenAlphabet[int(b)%len(captureTokenAlphabet)]
	}
	return string(out), nil
}

// verifyCaptureBinding loads the capture record identified by captureID and
// checks that its scenario, session, and surface match the current record
// request, and that one of the submitted artifacts has the same SHA256 as
// the captured artifact.
func verifyCaptureBinding(store *Store, run Run, scenarioID, sessionID, surface, captureID string, artifacts []Artifact) error {
	ledger := store.CaptureLedger(run)
	rec, ok, err := ledger.FindByID(captureID)
	if err != nil {
		return fmt.Errorf("load capture ledger: %w", err)
	}
	if !ok {
		return fmt.Errorf("capture id not found in ledger: %s", captureID)
	}
	if rec.Surface != surface {
		return fmt.Errorf("capture %s was for surface %s, cannot bind to %s evidence", captureID, rec.Surface, surface)
	}
	if rec.ScenarioID != scenarioID {
		return fmt.Errorf("capture %s belongs to scenario %s, cannot bind to scenario %s", captureID, rec.ScenarioID, scenarioID)
	}
	if rec.SessionID != sessionID {
		return fmt.Errorf("capture %s belongs to session %s, cannot bind to session %s", captureID, rec.SessionID, sessionID)
	}
	for _, art := range artifacts {
		if art.SHA256 == rec.ArtifactSHA256 {
			return nil
		}
	}
	return fmt.Errorf("no submitted artifact matches capture %s", captureID)
}

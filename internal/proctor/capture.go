package proctor

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	Nonce            string                  `json:"nonce"`
	ArtifactPath     string                  `json:"artifact_path"`
	ArtifactSHA256   string                  `json:"artifact_sha256"`
	ArtifactBytes    int64                   `json:"artifact_bytes"`
	TranscriptPath   string                  `json:"transcript_path,omitempty"`
	TranscriptSHA256 string                  `json:"transcript_sha256,omitempty"`
	Target           map[string]string       `json:"target"`
	Verification     CaptureVerificationMode `json:"verification"`
	CapturedAt       time.Time               `json:"captured_at"`
}

// CaptureOptions is the surface-agnostic input to capture dispatch.
type CaptureOptions struct {
	Surface    string
	ScenarioID string
	SessionID  string
	Label      string
	Target     map[string]string
}

// CaptureBackend is the interface each surface implements. Backends are registered
// via RegisterCaptureBackend, typically from init().
type CaptureBackend interface {
	// Capture performs the actual screenshot and any nonce injection. Backends write
	// the PNG (and optionally the transcript) to the provided destination paths and
	// return the surface-specific target metadata and verification mode.
	// The engine is responsible for filling ID, RunID, ScenarioID, SessionID, Label,
	// Surface, CapturedAt, ArtifactPath, ArtifactSHA256, ArtifactBytes, and any
	// transcript hash on the resulting ledger record.
	Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error)
}

// CaptureDestination tells the backend where to write artifacts and which nonce
// to plant on the target.
type CaptureDestination struct {
	ArtifactPath   string
	TranscriptPath string
	Nonce          string
}

// CaptureResult is what the backend returns after a successful capture.
type CaptureResult struct {
	Target            map[string]string
	Verification      CaptureVerificationMode
	TranscriptWritten bool
}

var (
	captureBackendsMu sync.RWMutex
	captureBackends   = map[string]CaptureBackend{}
)

// RegisterCaptureBackend registers a capture backend for the given surface.
// Real surface implementations register themselves in their own init()
// functions; the stub in capture_stubs.go only registers for surfaces that
// are still unbound after every other capture_*.go init has run.
func RegisterCaptureBackend(surface string, b CaptureBackend) {
	captureBackendsMu.Lock()
	defer captureBackendsMu.Unlock()
	captureBackends[surface] = b
}

func lookupCaptureBackend(surface string) (CaptureBackend, error) {
	captureBackendsMu.RLock()
	defer captureBackendsMu.RUnlock()
	backend, ok := captureBackends[surface]
	if !ok {
		return nil, fmt.Errorf("no capture backend registered for surface %q", surface)
	}
	return backend, nil
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

// Capture runs a capture for the given surface and writes the resulting record
// to the ledger. It generates the capture ID and nonce, prepares destination
// paths, invokes the backend, verifies the written artifact, and persists the
// committed record.
func Capture(ctx context.Context, store *Store, run Run, opts CaptureOptions) (CaptureRecord, error) {
	if store == nil {
		return CaptureRecord{}, fmt.Errorf("capture requires a store")
	}
	if opts.Surface == "" {
		return CaptureRecord{}, fmt.Errorf("capture requires a surface")
	}
	if opts.ScenarioID == "" {
		return CaptureRecord{}, fmt.Errorf("capture requires a scenario")
	}
	if opts.SessionID == "" {
		return CaptureRecord{}, fmt.Errorf("capture requires a session")
	}
	if _, ok := findScenario(run, opts.ScenarioID); !ok {
		return CaptureRecord{}, fmt.Errorf("unknown scenario: %s", opts.ScenarioID)
	}
	backend, err := lookupCaptureBackend(opts.Surface)
	if err != nil {
		return CaptureRecord{}, err
	}

	captureID, err := newCaptureID()
	if err != nil {
		return CaptureRecord{}, err
	}
	nonce, err := newCaptureNonce()
	if err != nil {
		return CaptureRecord{}, err
	}

	label := opts.Label
	if label == "" {
		label = "main"
	}
	labelSlug := slugify(label)

	artifactDir := filepath.Join(store.RunDir(run), "artifacts", opts.Surface, opts.ScenarioID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return CaptureRecord{}, err
	}
	artifactName := fmt.Sprintf("capture-%s-%s.png", captureID, labelSlug)
	artifactPath := filepath.Join(artifactDir, artifactName)

	transcriptPath := ""
	if opts.Surface == SurfaceCLI {
		transcriptPath = filepath.Join(artifactDir, fmt.Sprintf("capture-%s-%s.txt", captureID, labelSlug))
	}

	if ctx == nil {
		ctx = context.Background()
	}

	dest := CaptureDestination{
		ArtifactPath:   artifactPath,
		TranscriptPath: transcriptPath,
		Nonce:          nonce,
	}
	result, err := backend.Capture(ctx, dest, opts)
	if err != nil {
		return CaptureRecord{}, err
	}

	info, err := os.Stat(artifactPath)
	if err != nil {
		return CaptureRecord{}, fmt.Errorf("capture backend did not write artifact: %w", err)
	}
	if info.Size() < DefaultMinScreenshotSize {
		return CaptureRecord{}, fmt.Errorf("capture artifact is too small (%d bytes, minimum %d bytes)", info.Size(), DefaultMinScreenshotSize)
	}
	artifactSHA, err := hashFile(artifactPath)
	if err != nil {
		return CaptureRecord{}, err
	}

	transcriptSHA := ""
	if result.TranscriptWritten {
		if transcriptPath == "" {
			return CaptureRecord{}, fmt.Errorf("capture backend wrote a transcript but no transcript path was prepared")
		}
		tinfo, err := os.Stat(transcriptPath)
		if err != nil {
			return CaptureRecord{}, fmt.Errorf("capture backend did not write transcript: %w", err)
		}
		if tinfo.Size() < int64(DefaultMinTranscriptBytes) {
			return CaptureRecord{}, fmt.Errorf("capture transcript is too short (%d bytes, minimum %d bytes)", tinfo.Size(), DefaultMinTranscriptBytes)
		}
		transcriptSHA, err = hashFile(transcriptPath)
		if err != nil {
			return CaptureRecord{}, err
		}
	}

	verification := result.Verification
	if verification == "" {
		verification = CaptureVerifyNone
	}

	rec := CaptureRecord{
		ID:               captureID,
		RunID:            run.ID,
		ScenarioID:       opts.ScenarioID,
		SessionID:        opts.SessionID,
		Surface:          opts.Surface,
		Label:            label,
		Nonce:            nonce,
		ArtifactPath:     artifactPath,
		ArtifactSHA256:   artifactSHA,
		ArtifactBytes:    info.Size(),
		TranscriptPath:   "",
		TranscriptSHA256: transcriptSHA,
		Target:           result.Target,
		Verification:     verification,
		CapturedAt:       time.Now().UTC(),
	}
	if result.TranscriptWritten {
		rec.TranscriptPath = transcriptPath
	}

	ledger := store.CaptureLedger(run)
	if err := ledger.Append(rec); err != nil {
		return CaptureRecord{}, err
	}
	return rec, nil
}

// captureTokenAlphabet is the fixed character set used for capture IDs and nonces.
// It is upper-case alphanumeric to stay readable and template-matchable for the
// pixel verification backend.
const captureTokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// captureTokenLength is the fixed length for capture ID tokens and nonces.
const captureTokenLength = 6

// newCaptureID returns a fresh capture ID of the form "cap_XK7Q2M".
func newCaptureID() (string, error) {
	token, err := randomCaptureToken()
	if err != nil {
		return "", err
	}
	return "cap_" + token, nil
}

// newCaptureNonce returns a fresh 6-character nonce using crypto/rand.
func newCaptureNonce() (string, error) {
	return randomCaptureToken()
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

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
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

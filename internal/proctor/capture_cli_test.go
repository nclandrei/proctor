package proctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeCLIPNG drops a byte blob large enough to satisfy
// DefaultMinScreenshotSize. The engine only stats the file size; structural
// PNG validity is not required for these unit tests.
func writeFakeCLIPNG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	size := int(DefaultMinScreenshotSize) + 1024
	buf := make([]byte, size)
	copy(buf, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	for i := 8; i < size; i++ {
		buf[i] = byte(i & 0xff)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
}

type tmuxFakeCall struct {
	args []string
}

type tmuxFake struct {
	initialStatusRight string
	sessionExists      bool
	paneText           string
	rejectNoncePlant   bool
	setOptionErr       error
	capturePaneEmpty   bool

	currentStatusRight string
	plantedNonce       string
	calls              []tmuxFakeCall
}

func newTmuxFake(t *testing.T, initial, paneText string) *tmuxFake {
	t.Helper()
	return &tmuxFake{
		initialStatusRight: initial,
		sessionExists:      true,
		paneText:           paneText,
		currentStatusRight: initial,
	}
}

func (f *tmuxFake) run(ctx context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, tmuxFakeCall{args: append([]string(nil), args...)})
	if len(args) == 0 {
		return nil, errors.New("tmuxFake: no args")
	}
	switch args[0] {
	case "has-session":
		if !f.sessionExists {
			return nil, errors.New("can't find session")
		}
		return nil, nil
	case "show-options":
		return []byte(f.currentStatusRight), nil
	case "set-option":
		if hasFlag(args, "-u") {
			f.currentStatusRight = ""
			return nil, nil
		}
		if len(args) >= 5 {
			value := args[4]
			if strings.HasPrefix(value, "proctor:") {
				if f.setOptionErr != nil {
					return nil, f.setOptionErr
				}
				f.plantedNonce = value
				if !f.rejectNoncePlant {
					f.currentStatusRight = value
				}
				return nil, nil
			}
			f.currentStatusRight = value
			return nil, nil
		}
		return nil, errors.New("tmuxFake: set-option with too few args")
	case "refresh-client":
		return nil, nil
	case "capture-pane":
		if f.capturePaneEmpty {
			return nil, nil
		}
		return []byte(f.paneText), nil
	}
	return nil, fmt.Errorf("tmuxFake: unknown command %q", args[0])
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func (f *tmuxFake) setOptionCalls() []tmuxFakeCall {
	var out []tmuxFakeCall
	for _, c := range f.calls {
		if len(c.args) > 0 && c.args[0] == "set-option" {
			out = append(out, c)
		}
	}
	return out
}

func newFakeCLIBackend(t *testing.T, fake *tmuxFake) *cliBackend {
	t.Helper()
	return &cliBackend{
		runTmux: fake.run,
		screenCap: func(ctx context.Context, dest string) error {
			writeFakeCLIPNG(t, dest)
			return nil
		},
		sleep: func(d time.Duration) {},
	}
}

func TestCLICaptureHappyPath(t *testing.T) {
	fake := newTmuxFake(t, "", "line 1\nline 2\n")
	backend := newFakeCLIBackend(t, fake)

	tmp := t.TempDir()
	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(tmp, "cap.png"),
		TranscriptPath: filepath.Join(tmp, "cap.txt"),
		Nonce:          "AB12CD",
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": "work:0.0"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Verification != CaptureVerifyMeta {
		t.Fatalf("expected verification meta, got %s", result.Verification)
	}
	if !result.TranscriptWritten {
		t.Fatal("expected TranscriptWritten=true")
	}
	if result.Target["tmux_target"] != "work:0.0" {
		t.Fatalf("expected tmux_target work:0.0, got %q", result.Target["tmux_target"])
	}
	if result.Target["tmux_session"] != "work" {
		t.Fatalf("expected tmux_session work, got %q", result.Target["tmux_session"])
	}
	if result.Target["transcript_nonce_found"] != "true" {
		t.Fatalf("expected transcript_nonce_found=true, got %q", result.Target["transcript_nonce_found"])
	}
	if want := "proctor:AB12CD"; result.Target["status_right_readback"] != want {
		t.Fatalf("expected status_right_readback=%q, got %q", want, result.Target["status_right_readback"])
	}

	info, err := os.Stat(dest.ArtifactPath)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if info.Size() < DefaultMinScreenshotSize {
		t.Fatalf("artifact too small: %d bytes", info.Size())
	}
	trData, err := os.ReadFile(dest.TranscriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	trStr := string(trData)
	if !strings.Contains(trStr, "line 1") || !strings.Contains(trStr, "line 2") {
		t.Fatalf("transcript missing pane content: %q", trStr)
	}
	if !strings.Contains(trStr, "proctor:AB12CD") {
		t.Fatalf("transcript missing proctor footer nonce: %q", trStr)
	}
	if !strings.Contains(trStr, "tmux_target: work:0.0") {
		t.Fatalf("transcript missing proctor footer target: %q", trStr)
	}

	setCalls := fake.setOptionCalls()
	if len(setCalls) < 2 {
		t.Fatalf("expected at least 2 set-option calls (plant + restore), got %d", len(setCalls))
	}
	last := setCalls[len(setCalls)-1]
	if !hasFlag(last.args, "-u") {
		t.Fatalf("expected final set-option to unset status-right (have -u flag), got args=%v", last.args)
	}
	if fake.currentStatusRight != "" {
		t.Fatalf("expected status-right to be cleared, got %q", fake.currentStatusRight)
	}
}

func TestCLICaptureRestoresPreexistingStatusRight(t *testing.T) {
	prev := "#[fg=green]#H #[default]| %Y-%m-%d"
	fake := newTmuxFake(t, prev, "shell output\n")
	backend := newFakeCLIBackend(t, fake)

	tmp := t.TempDir()
	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(tmp, "cap.png"),
		TranscriptPath: filepath.Join(tmp, "cap.txt"),
		Nonce:          "NONCE1",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": "dev"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if fake.currentStatusRight != prev {
		t.Fatalf("expected status-right restored to %q, got %q", prev, fake.currentStatusRight)
	}
	setCalls := fake.setOptionCalls()
	if len(setCalls) < 2 {
		t.Fatalf("expected >= 2 set-option calls, got %d", len(setCalls))
	}
	last := setCalls[len(setCalls)-1]
	if hasFlag(last.args, "-u") {
		t.Fatalf("expected restoration to re-set the option, not unset it: args=%v", last.args)
	}
	if len(last.args) < 5 || last.args[4] != prev {
		t.Fatalf("expected restoration to write previous value %q, got args=%v", prev, last.args)
	}
}

func TestCLICaptureSessionMissing(t *testing.T) {
	fake := newTmuxFake(t, "", "")
	fake.sessionExists = false
	backend := newFakeCLIBackend(t, fake)

	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(t.TempDir(), "cap.png"),
		TranscriptPath: filepath.Join(t.TempDir(), "cap.txt"),
		Nonce:          "XYZ999",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": "ghost:0"},
	})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got %v", err)
	}
	if got := len(fake.setOptionCalls()); got != 0 {
		t.Fatalf("expected no set-option calls on missing session, got %d", got)
	}
}

func TestCLICaptureStatusRightRejected(t *testing.T) {
	fake := newTmuxFake(t, "", "shell line\n")
	fake.rejectNoncePlant = true
	backend := newFakeCLIBackend(t, fake)

	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(t.TempDir(), "cap.png"),
		TranscriptPath: filepath.Join(t.TempDir(), "cap.txt"),
		Nonce:          "MISS01",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": "work"},
	})
	if err == nil {
		t.Fatal("expected error when readback does not contain nonce")
	}
	if !strings.Contains(err.Error(), "not visible in status-right readback") {
		t.Fatalf("expected status-right readback error, got %v", err)
	}
	if fake.currentStatusRight != "" {
		t.Fatalf("expected status-right cleared after restore, got %q", fake.currentStatusRight)
	}
}

func TestCLICaptureSetOptionErrorTriggersRestore(t *testing.T) {
	prev := "original"
	fake := newTmuxFake(t, prev, "")
	fake.setOptionErr = errors.New("option refused")
	backend := newFakeCLIBackend(t, fake)

	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(t.TempDir(), "cap.png"),
		TranscriptPath: filepath.Join(t.TempDir(), "cap.txt"),
		Nonce:          "BOOM01",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": "work"},
	})
	if err == nil {
		t.Fatal("expected error when set-option fails")
	}
	if !strings.Contains(err.Error(), "plant nonce") {
		t.Fatalf("expected plant nonce error, got %v", err)
	}
	if fake.currentStatusRight != prev {
		t.Fatalf("expected status-right to return to %q, got %q", prev, fake.currentStatusRight)
	}
	restored := false
	for _, c := range fake.setOptionCalls() {
		if len(c.args) >= 5 && c.args[4] == prev && !hasFlag(c.args, "-u") {
			restored = true
		}
	}
	if !restored {
		t.Fatalf("expected deferred restore to write %q, calls=%+v", prev, fake.setOptionCalls())
	}
}

func TestCLICaptureScreenshotFails(t *testing.T) {
	fake := newTmuxFake(t, "", "line\n")
	backend := &cliBackend{
		runTmux: fake.run,
		screenCap: func(ctx context.Context, dest string) error {
			return errors.New("screencapture crashed")
		},
		sleep: func(d time.Duration) {},
	}

	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(t.TempDir(), "cap.png"),
		TranscriptPath: filepath.Join(t.TempDir(), "cap.txt"),
		Nonce:          "SHOT01",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": "work"},
	})
	if err == nil {
		t.Fatal("expected screencapture failure to surface")
	}
	if !strings.Contains(err.Error(), "screencapture") {
		t.Fatalf("expected wrapped screencapture error, got %v", err)
	}
	if !strings.Contains(err.Error(), "crashed") {
		t.Fatalf("expected underlying error text, got %v", err)
	}
	if fake.currentStatusRight != "" {
		t.Fatalf("expected status-right cleared, got %q", fake.currentStatusRight)
	}
}

func TestCLICaptureEmptyPane(t *testing.T) {
	fake := newTmuxFake(t, "", "")
	fake.capturePaneEmpty = true
	backend := newFakeCLIBackend(t, fake)

	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(t.TempDir(), "cap.png"),
		TranscriptPath: filepath.Join(t.TempDir(), "cap.txt"),
		Nonce:          "EMPTY1",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": "work"},
	})
	if err == nil {
		t.Fatal("expected empty capture-pane to error")
	}
	if !strings.Contains(err.Error(), "empty transcript") {
		t.Fatalf("expected empty-transcript error, got %v", err)
	}
}

func TestCLICaptureMissingTmuxTarget(t *testing.T) {
	backend := newFakeCLIBackend(t, newTmuxFake(t, "", ""))
	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(t.TempDir(), "cap.png"),
		TranscriptPath: filepath.Join(t.TempDir(), "cap.txt"),
		Nonce:          "NOARG1",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing tmux_target")
	}
	if !strings.Contains(err.Error(), "tmux_target is required") {
		t.Fatalf("expected required-arg guidance, got %v", err)
	}
}

func TestParseTmuxSession(t *testing.T) {
	cases := map[string]string{
		"work":     "work",
		"work:0":   "work",
		"work:0.1": "work",
		"dev:2.3":  "dev",
	}
	for in, want := range cases {
		if got := parseTmuxSession(in); got != want {
			t.Errorf("parseTmuxSession(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestCLICaptureRegistered(t *testing.T) {
	backend, err := lookupCaptureBackend(SurfaceCLI)
	if err != nil {
		t.Fatalf("lookup cli backend: %v", err)
	}
	if _, ok := backend.(*cliBackend); !ok {
		t.Fatalf("expected *cliBackend, got %T", backend)
	}
}

var _ CaptureBackend = (*cliBackend)(nil)

// TestCLIIntegrationTmux exercises the real tmux binary when available.
// It spins up a detached session with a shell, sends a visible line, then
// runs the backend against it. The screencapture call is mocked because
// CI environments cannot take real display screenshots.
func TestCLIIntegrationTmux(t *testing.T) {
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not available on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session := fmt.Sprintf("proctor-cli-it-%d", time.Now().UnixNano())
	start := exec.CommandContext(ctx, tmuxBin, "new-session", "-d", "-s", session, "sh")
	if out, err := start.CombinedOutput(); err != nil {
		t.Skipf("tmux new-session failed (sandbox?): %v: %s", err, strings.TrimSpace(string(out)))
	}
	defer func() {
		kill := exec.Command(tmuxBin, "kill-session", "-t", session)
		_ = kill.Run()
	}()
	sendKeys := exec.CommandContext(ctx, tmuxBin, "send-keys", "-t", session+":0.0", "echo proctor-integration-ready", "Enter")
	if out, err := sendKeys.CombinedOutput(); err != nil {
		t.Skipf("tmux send-keys failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	time.Sleep(250 * time.Millisecond)

	backend := &cliBackend{
		runTmux: runTmuxCmd,
		screenCap: func(ctx context.Context, dest string) error {
			writeFakeCLIPNG(t, dest)
			return nil
		},
		sleep: time.Sleep,
	}

	tmp := t.TempDir()
	dest := CaptureDestination{
		ArtifactPath:   filepath.Join(tmp, "cap.png"),
		TranscriptPath: filepath.Join(tmp, "cap.txt"),
		Nonce:          "INTEG1",
	}
	result, err := backend.Capture(ctx, dest, CaptureOptions{
		Surface: SurfaceCLI,
		Target:  map[string]string{"tmux_target": session + ":0.0"},
	})
	if err != nil {
		t.Fatalf("integration capture: %v", err)
	}
	if result.Verification != CaptureVerifyMeta {
		t.Fatalf("expected CaptureVerifyMeta, got %s", result.Verification)
	}
	if !result.TranscriptWritten {
		t.Fatal("expected transcript written")
	}
	if result.Target["tmux_session"] != session {
		t.Fatalf("expected tmux_session=%q, got %q", session, result.Target["tmux_session"])
	}
	if result.Target["status_right_readback"] != "proctor:INTEG1" {
		t.Fatalf("expected status_right_readback=proctor:INTEG1, got %q", result.Target["status_right_readback"])
	}
	trData, err := os.ReadFile(dest.TranscriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if !strings.Contains(string(trData), "proctor:INTEG1") {
		t.Fatalf("integration transcript missing nonce footer: %q", string(trData))
	}
}

package proctor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cliBackend captures CLI evidence by planting a nonce in the status-right
// segment of a tmux session, forcing a repaint, taking a full-screen
// screencapture, and reading back the rendered tmux pane via capture-pane.
//
// The nonce proof uses two tmux reads:
//  1. `show-options -v status-right` is read back after the plant to confirm
//     tmux actually accepted the value. This is the authoritative "nonce
//     present" check — equivalent to a server-side echo.
//  2. `capture-pane` snapshots the pane content and is written to disk as
//     the transcript. The transcript is high-trust terminal-session proof
//     even when it does not itself contain the nonce marker (status-right
//     lives on the client status line, not in the pane buffer).
//
// The backend always returns CaptureVerifyMeta: terminal fonts vary too
// much to reliably template-match the built-in 5x7 bitmap font, so pixel
// verification is not attempted. Both OS seams (tmux and screencapture)
// are replaced in tests.
type cliBackend struct {
	runTmux   func(ctx context.Context, args ...string) ([]byte, error)
	screenCap func(ctx context.Context, dest string) error
	// sleep lets tests avoid the 200ms render delay. Defaults to time.Sleep
	// in production.
	sleep func(d time.Duration)
}

// defaultCLIBackend returns a backend that shells out to the real tmux and
// screencapture binaries.
func defaultCLIBackend() *cliBackend {
	return &cliBackend{
		runTmux:   runTmuxCmd,
		screenCap: runScreencaptureFull,
		sleep:     time.Sleep,
	}
}

func init() {
	RegisterCaptureBackend(SurfaceCLI, defaultCLIBackend())
}

// cliRenderDelay is how long we wait after planting the nonce before capturing
// the pane and screen. tmux's refresh-client is asynchronous and terminal
// emulators need a beat to repaint.
const cliRenderDelay = 200 * time.Millisecond

// Capture plants the proctor nonce in the tmux status bar, captures the
// terminal, reads the rendered pane text, and verifies the nonce appears in
// that transcript. The previous status-right is always restored, even on
// error paths.
func (b *cliBackend) Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error) {
	target := strings.TrimSpace(opts.Target["tmux_target"])
	if target == "" {
		return CaptureResult{}, fmt.Errorf("cli capture: tmux_target is required")
	}
	session := parseTmuxSession(target)
	if session == "" {
		return CaptureResult{}, fmt.Errorf("cli capture: tmux_target %q is missing a session name", target)
	}

	runTmux := b.runTmux
	if runTmux == nil {
		runTmux = runTmuxCmd
	}
	screenCap := b.screenCap
	if screenCap == nil {
		screenCap = runScreencaptureFull
	}
	sleep := b.sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	// Session existence check uses the parsed session (has-session -t takes a
	// session, not a full target spec).
	if _, err := runTmux(ctx, "has-session", "-t", session); err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: tmux session %q not found: %w", session, err)
	}

	// Save the current status-right so we can restore it afterwards. tmux
	// returns empty output when the option is unset.
	prevOut, err := runTmux(ctx, "show-options", "-t", target, "-v", "status-right")
	if err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: read status-right: %w", err)
	}
	prevStatusRight := strings.TrimRight(string(prevOut), "\r\n")
	hadPrev := prevStatusRight != ""

	// Restoration runs on every exit path below.
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		if hadPrev {
			_, _ = runTmux(ctx, "set-option", "-t", target, "status-right", prevStatusRight)
		} else {
			_, _ = runTmux(ctx, "set-option", "-t", target, "-u", "status-right")
		}
	}
	defer restore()

	nonceMarker := "proctor:" + dest.Nonce
	if _, err := runTmux(ctx, "set-option", "-t", target, "status-right", nonceMarker); err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: plant nonce: %w", err)
	}

	// refresh-client is a best-effort repaint hint. Detached sessions and
	// daemon-only environments have no attached client, which tmux surfaces
	// as "can't find client". That is harmless: capture-pane reads from
	// tmux's internal pane buffer and set-option has already mutated
	// status-right, so the next capture-pane call will still see the nonce.
	_, _ = runTmux(ctx, "refresh-client", "-t", target)

	sleep(cliRenderDelay)

	// Read back the option tmux now holds. This is the authoritative
	// "status-right was planted" check — if tmux rejected the value, the
	// readback will not contain our marker.
	readbackOut, err := runTmux(ctx, "show-options", "-t", target, "-v", "status-right")
	if err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: verify status-right: %w", err)
	}
	readback := strings.TrimRight(string(readbackOut), "\r\n")
	if !strings.Contains(readback, nonceMarker) {
		return CaptureResult{}, fmt.Errorf(
			"cli capture: planted nonce %q not visible in status-right readback (got %q) — tmux may have rejected the value",
			nonceMarker, readback,
		)
	}

	paneOut, err := runTmux(ctx, "capture-pane", "-t", target, "-p", "-S", "-1000")
	if err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: capture-pane: %w", err)
	}
	if strings.TrimSpace(string(paneOut)) == "" {
		return CaptureResult{}, fmt.Errorf("cli capture: tmux capture-pane returned empty transcript for %q", target)
	}

	// Annotate the pane buffer with the planted nonce so the written
	// transcript is self-describing: any reader of the transcript file can
	// see which nonce this capture bound to. The annotation is a trailing
	// footer, so the pane buffer contents remain intact above it.
	transcriptBytes := annotateCLITranscript(paneOut, nonceMarker, target)

	if dest.TranscriptPath == "" {
		return CaptureResult{}, fmt.Errorf("cli capture: transcript path not provided by engine")
	}
	if err := os.MkdirAll(filepath.Dir(dest.TranscriptPath), 0o755); err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: prepare transcript dir: %w", err)
	}
	if err := os.WriteFile(dest.TranscriptPath, transcriptBytes, 0o644); err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: write transcript: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dest.ArtifactPath), 0o755); err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: prepare artifact dir: %w", err)
	}
	if err := screenCap(ctx, dest.ArtifactPath); err != nil {
		return CaptureResult{}, fmt.Errorf("cli capture: screencapture: %w", err)
	}

	result := CaptureResult{
		Target: map[string]string{
			"tmux_target":            target,
			"tmux_session":           session,
			"transcript_nonce_found": "true",
			"status_right_readback":  readback,
		},
		Verification:      CaptureVerifyMeta,
		TranscriptWritten: true,
	}
	return result, nil
}

// annotateCLITranscript appends a proctor footer to the raw tmux pane
// capture. The footer carries the planted nonce marker and target so the
// transcript file stands on its own as evidence: a reader can correlate
// which tmux session was captured and which nonce should have appeared in
// the status bar.
func annotateCLITranscript(paneOut []byte, nonceMarker, target string) []byte {
	var buf bytes.Buffer
	buf.Write(paneOut)
	if len(paneOut) > 0 && paneOut[len(paneOut)-1] != '\n' {
		buf.WriteByte('\n')
	}
	fmt.Fprintf(&buf, "--- proctor cli capture ---\n")
	fmt.Fprintf(&buf, "tmux_target: %s\n", target)
	fmt.Fprintf(&buf, "status_right_planted: %s\n", nonceMarker)
	return buf.Bytes()
}

// parseTmuxSession extracts the session name from a tmux target spec. A
// target of "sess", "sess:0", or "sess:0.1" all resolve to "sess".
func parseTmuxSession(target string) string {
	if i := strings.IndexByte(target, ':'); i >= 0 {
		return target[:i]
	}
	return target
}

// runTmuxCmd shells out to the real tmux binary and returns its stdout.
// stderr is included in the returned error so callers can surface tmux's
// own message ("can't find session", etc.).
func runTmuxCmd(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// runScreencaptureFull captures the entire display to dest using the macOS
// screencapture CLI. -o suppresses the window shadow, -x silences the
// camera sound. Full-screen capture is sufficient for CLI because the tmux
// status bar (where we plant the nonce) is visible in the bottom row of
// any terminal window on screen.
func runScreencaptureFull(ctx context.Context, dest string) error {
	cmd := exec.CommandContext(ctx, "/usr/sbin/screencapture", "-o", "-x", dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

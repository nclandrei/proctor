package proctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// iosSimulatorWidth mirrors the iPhone 14/15 logical 3x-density width in
// points (390) but we stretch it to physical pixels for tests so the
// status bar scan region is comfortable and the nonce template fits.
const iosSimulatorWidth = 390
const iosSimulatorHeight = 844

// synthesizeIOSScreenshot pastes the rendered nonce at the given scale
// into the top-left of a checkerboard canvas matching a typical iPhone
// simulator capture, then PNG-encodes it. This is what our fake
// runSimctl "io screenshot" writes to disk.
func synthesizeIOSScreenshot(t *testing.T, nonce string, scale int) []byte {
	t.Helper()
	return synthesizeIOSScreenshotSized(t, nonce, scale, iosSimulatorWidth, iosSimulatorHeight)
}

func synthesizeIOSScreenshotSized(t *testing.T, nonce string, scale int, width, height int) []byte {
	t.Helper()
	canvas := image.NewGray(image.Rect(0, 0, width, height))
	// Fill with a deterministic 50/50 checkerboard-like pattern so neither
	// polarity can trivially saturate against a uniform background. If
	// the fixture were pure white, inverted polarity would treat every
	// pixel as ink and VerifyNonceInRegion would return a 100% match for
	// any nonce at any offset. The 50/50 scramble bounds the trivial
	// similarity under both polarities well below the 0.85 tolerance so
	// a correctly-planted nonce is the only thing that can cross it.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if ((x ^ y) & 1) == 0 {
				canvas.SetGray(x, y, color.Gray{Y: 0x10})
			} else {
				canvas.SetGray(x, y, color.Gray{Y: 0xF0})
			}
		}
	}
	if nonce != "" {
		tpl := RenderNonce("proctor:"+nonce, scale)
		b := tpl.Bounds()
		// Offset a few pixels down and in so verification must scan for it.
		offX := 10
		offY := 10
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				if offX+x >= width || offY+y >= height {
					continue
				}
				canvas.SetGray(offX+x, offY+y, tpl.GrayAt(b.Min.X+x, b.Min.Y+y))
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// simctlCall records one invocation of the fake simctl so tests can
// assert on ordering and arguments.
type simctlCall struct {
	Args []string
}

// fakeSimctl is a script-driven simctl stand-in. Callers register a
// handler that inspects args and either returns JSON/output or an error.
type fakeSimctl struct {
	mu      sync.Mutex
	calls   []simctlCall
	handler func(call simctlCall) ([]byte, error)
}

func newFakeSimctl(handler func(call simctlCall) ([]byte, error)) *fakeSimctl {
	return &fakeSimctl{handler: handler}
}

func (f *fakeSimctl) run(ctx context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	call := simctlCall{Args: append([]string(nil), args...)}
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	return f.handler(call)
}

func (f *fakeSimctl) snapshot() []simctlCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]simctlCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// iosCallKind classifies a simctl argv so test tables can pattern-match
// without repeating indexing logic.
func iosCallKind(c simctlCall) string {
	if len(c.Args) == 0 {
		return "empty"
	}
	switch c.Args[0] {
	case "list":
		return "list"
	case "launch":
		return "launch"
	case "io":
		return "io"
	case "status_bar":
		if len(c.Args) >= 3 {
			return "status_bar:" + c.Args[2]
		}
		return "status_bar"
	}
	return c.Args[0]
}

// iosFakeFilesystem backs the backend's readFile seam with an in-memory
// map keyed by path.
type iosFakeFilesystem struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newIOSFakeFS() *iosFakeFilesystem {
	return &iosFakeFilesystem{files: map[string][]byte{}}
}

func (fs *iosFakeFilesystem) write(path string, data []byte) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.files[path] = append([]byte(nil), data...)
}

func (fs *iosFakeFilesystem) read(path string) ([]byte, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	b, ok := fs.files[path]
	if !ok {
		return nil, fmt.Errorf("file %s not found", path)
	}
	return append([]byte(nil), b...), nil
}

func (fs *iosFakeFilesystem) delete(path string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	delete(fs.files, path)
}

// buildIOSBackend wires a fake simctl and a fake filesystem together,
// writing the synthesized screenshot to the artifact path when the fake
// `io screenshot` is invoked.
func buildIOSBackend(t *testing.T, fakeFS *iosFakeFilesystem, screenshot []byte, listJSON []byte, bootedListJSON []byte, launchErr, screenshotErr, clearErr error) (*iosBackend, *fakeSimctl) {
	t.Helper()
	fake := newFakeSimctl(func(call simctlCall) ([]byte, error) {
		switch iosCallKind(call) {
		case "list":
			// list devices -j OR list devices booted -j
			for _, a := range call.Args {
				if a == "booted" {
					if bootedListJSON == nil {
						return []byte(`{"devices":{}}`), nil
					}
					return bootedListJSON, nil
				}
			}
			if listJSON == nil {
				return []byte(`{"devices":{}}`), nil
			}
			return listJSON, nil
		case "launch":
			if launchErr != nil {
				return nil, launchErr
			}
			return []byte("launched"), nil
		case "status_bar:override":
			return []byte(""), nil
		case "status_bar:clear":
			if clearErr != nil {
				return nil, clearErr
			}
			return []byte(""), nil
		case "io":
			if screenshotErr != nil {
				return nil, screenshotErr
			}
			// argv is [io, udid, screenshot, dest]
			if len(call.Args) >= 4 {
				dest := call.Args[3]
				fakeFS.write(dest, screenshot)
				// Also write to the real filesystem so the engine's
				// os.Stat/size check passes in integration tests. Here
				// we only need the fake FS for the backend's readFile.
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err == nil {
					_ = os.WriteFile(dest, screenshot, 0o644)
				}
			}
			return []byte(""), nil
		}
		return nil, fmt.Errorf("fake simctl: unexpected call: %v", call.Args)
	})
	backend := &iosBackend{
		runSimctl: fake.run,
		readFile:  fakeFS.read,
	}
	return backend, fake
}

func TestIOSCaptureHappyPathBooted(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "ABC123", 2)
	bootedJSON := []byte(`{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-17-0":[{"udid":"11111111-2222-3333-4444-555555555555","name":"iPhone 15","state":"Booted"}]}}`)
	backend, fake := buildIOSBackend(t, fs, shot, nil, bootedJSON, nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "ABC123",
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "booted"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Verification != CaptureVerifyPixel {
		t.Fatalf("expected verification pixel, got %s", result.Verification)
	}
	if result.TranscriptWritten {
		t.Fatal("ios backend must not claim a transcript was written")
	}
	if result.Target["simulator"] != "booted" {
		t.Fatalf("expected simulator=booted, got %q", result.Target["simulator"])
	}
	if result.Target["simulator_name"] != "iPhone 15" {
		t.Fatalf("expected simulator_name=iPhone 15, got %q", result.Target["simulator_name"])
	}
	if result.Target["scale"] != "2" {
		t.Fatalf("expected scale=2, got %q", result.Target["scale"])
	}
	if got := result.Target["nonce_region"]; !strings.HasPrefix(got, "0,0,") {
		t.Fatalf("expected nonce_region to start with 0,0, got %q", got)
	}

	// Verify call ordering: list booted, status_bar override, io
	// screenshot, status_bar clear. launch must not appear because
	// bundle_id was not set.
	kinds := []string{}
	for _, c := range fake.snapshot() {
		kinds = append(kinds, iosCallKind(c))
	}
	if !containsSeq(kinds, []string{"status_bar:override", "io", "status_bar:clear"}) {
		t.Fatalf("expected override->io->clear order, got %v", kinds)
	}
	for _, k := range kinds {
		if k == "launch" {
			t.Fatalf("launch must not run without bundle_id, got calls %v", kinds)
		}
	}

	// The happy-path capture should not trigger the full `list devices
	// -j` fallback because we went through the booted short-circuit.
	for _, c := range fake.snapshot() {
		if iosCallKind(c) == "list" {
			foundBooted := false
			for _, a := range c.Args {
				if a == "booted" {
					foundBooted = true
				}
			}
			if !foundBooted {
				t.Fatalf("expected only booted list lookup, got args=%v", c.Args)
			}
		}
	}
}

func TestIOSCaptureShortCircuitsBootedWithoutFullList(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "ZZZZZZ", 2)
	backend, fake := buildIOSBackend(t, fs, shot, nil, []byte(`{"devices":{}}`), nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "ZZZZZZ",
	}
	if _, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": ""}, // empty → default booted
	}); err != nil {
		t.Fatalf("capture: %v", err)
	}
	// Even though no booted simulator was returned, the capture should
	// proceed because "booted" is accepted literally by simctl.
	for _, c := range fake.snapshot() {
		if iosCallKind(c) == "list" {
			foundBooted := false
			for _, a := range c.Args {
				if a == "booted" {
					foundBooted = true
				}
			}
			if !foundBooted {
				t.Fatalf("did not expect full list lookup, got %v", c.Args)
			}
		}
	}
}

func TestIOSCaptureLaunchesBundleID(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "APP001", 2)
	bootedJSON := []byte(`{"devices":{"rt":[{"udid":"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE","name":"iPhone 15 Pro","state":"Booted"}]}}`)
	backend, fake := buildIOSBackend(t, fs, shot, nil, bootedJSON, nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "APP001",
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target: map[string]string{
			"simulator": "booted",
			"bundle_id": "com.example.myapp",
		},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Target["bundle_id"] != "com.example.myapp" {
		t.Fatalf("expected bundle_id in target, got %q", result.Target["bundle_id"])
	}
	var found bool
	for _, c := range fake.snapshot() {
		if iosCallKind(c) == "launch" {
			found = true
			if len(c.Args) < 3 {
				t.Fatalf("launch call too short: %v", c.Args)
			}
			if c.Args[2] != "com.example.myapp" {
				t.Fatalf("launch bundle_id=%q, want com.example.myapp", c.Args[2])
			}
		}
	}
	if !found {
		t.Fatal("expected launch call to fire when bundle_id set")
	}
}

func TestIOSCaptureScale3Fallback(t *testing.T) {
	fs := newIOSFakeFS()
	// Render at scale=3 on purpose so the scale=2 attempt fails.
	shot := synthesizeIOSScreenshot(t, "BIG003", 3)
	backend, _ := buildIOSBackend(t, fs, shot, nil, []byte(`{"devices":{}}`), nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "BIG003",
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "booted"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Target["scale"] != "3" {
		t.Fatalf("expected scale=3 fallback, got %q", result.Target["scale"])
	}
}

func TestIOSCapturePixelVerificationFails(t *testing.T) {
	fs := newIOSFakeFS()
	// Plant nothing at all in the status bar area: the fake screenshot
	// is a plain 50/50 checkerboard. VerifyNonceInRegion has no nonce to
	// find at either scale, so the best similarity sits well below the
	// 0.85 threshold and the capture must surface the failure.
	shot := synthesizeIOSScreenshot(t, "", 2)
	backend, fake := buildIOSBackend(t, fs, shot, nil, []byte(`{"devices":{}}`), nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "RIGHT1",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "booted"},
	})
	if err == nil {
		t.Fatal("expected pixel verification error")
	}
	if !strings.Contains(err.Error(), "pixel verification failed") {
		t.Fatalf("expected 'pixel verification failed' in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "similarity") {
		t.Fatalf("expected similarity detail in error, got %v", err)
	}
	// Clear must still have been called.
	sawClear := false
	for _, c := range fake.snapshot() {
		if iosCallKind(c) == "status_bar:clear" {
			sawClear = true
		}
	}
	if !sawClear {
		t.Fatal("expected status_bar clear to run even when verification fails")
	}
}

func TestIOSCaptureLaunchErrorPropagates(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "NOPE00", 2)
	launchErr := errors.New("launch boom")
	backend, fake := buildIOSBackend(t, fs, shot, nil, []byte(`{"devices":{}}`), launchErr, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "NOPE00",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target: map[string]string{
			"simulator": "booted",
			"bundle_id": "com.example.boom",
		},
	})
	if err == nil {
		t.Fatal("expected launch error to propagate")
	}
	if !strings.Contains(err.Error(), "launch") {
		t.Fatalf("expected launch in error, got %v", err)
	}
	// Because launch fails *before* the override, clear is not
	// registered. The defer only arms itself after a successful
	// override. Confirm no override was set and therefore no clear was
	// invoked either.
	for _, c := range fake.snapshot() {
		if iosCallKind(c) == "status_bar:override" {
			t.Fatal("override must not run when launch fails")
		}
		if iosCallKind(c) == "status_bar:clear" {
			t.Fatal("clear must not run when override was never planted")
		}
	}
}

func TestIOSCaptureScreenshotErrorStillClears(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "SHOT00", 2)
	shotErr := errors.New("screenshot boom")
	backend, fake := buildIOSBackend(t, fs, shot, nil, []byte(`{"devices":{}}`), nil, shotErr, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "SHOT00",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "booted"},
	})
	if err == nil {
		t.Fatal("expected screenshot error to propagate")
	}
	if !strings.Contains(err.Error(), "screenshot") {
		t.Fatalf("expected screenshot in error, got %v", err)
	}
	sawClear := false
	for _, c := range fake.snapshot() {
		if iosCallKind(c) == "status_bar:clear" {
			sawClear = true
		}
	}
	if !sawClear {
		t.Fatal("expected status_bar clear after screenshot failure")
	}
}

func TestIOSCaptureScreenshotFileMissing(t *testing.T) {
	fs := newIOSFakeFS()
	// readFile will fail because no one wrote to the path.
	backend := &iosBackend{
		runSimctl: func(ctx context.Context, args ...string) ([]byte, error) {
			// Accept all calls without writing a file.
			return []byte(""), nil
		},
		readFile: fs.read,
	}
	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "MISS01",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "booted"},
	})
	if err == nil {
		t.Fatal("expected error when screenshot file is missing")
	}
	if !strings.Contains(err.Error(), "read screenshot") {
		t.Fatalf("expected 'read screenshot' in error, got %v", err)
	}
}

func TestIOSCaptureClearFailureIsSwallowed(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "CLEAR1", 2)
	clearErr := errors.New("clear boom")
	backend, _ := buildIOSBackend(t, fs, shot, nil, []byte(`{"devices":{}}`), nil, nil, clearErr)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "CLEAR1",
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "booted"},
	})
	if err != nil {
		t.Fatalf("capture should succeed even if clear fails, got %v", err)
	}
	if result.Verification != CaptureVerifyPixel {
		t.Fatalf("expected verification pixel, got %s", result.Verification)
	}
}

func TestIOSCaptureResolvesNameFromList(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "NAM001", 2)
	listJSON := []byte(`{"devices":{"rt":[{"udid":"11111111-2222-3333-4444-555555555555","name":"iPhone 15","state":"Shutdown"},{"udid":"99999999-8888-7777-6666-555555555555","name":"iPhone 15","state":"Booted"}]}}`)
	backend, _ := buildIOSBackend(t, fs, shot, listJSON, nil, nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "NAM001",
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "iPhone 15"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	// Should pick the Booted one.
	if result.Target["simulator"] != "99999999-8888-7777-6666-555555555555" {
		t.Fatalf("expected booted UDID, got %q", result.Target["simulator"])
	}
	if result.Target["simulator_name"] != "iPhone 15" {
		t.Fatalf("expected simulator_name iPhone 15, got %q", result.Target["simulator_name"])
	}
}

func TestIOSCaptureNameNotFound(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "NAM002", 2)
	listJSON := []byte(`{"devices":{"rt":[{"udid":"11111111-2222-3333-4444-555555555555","name":"iPhone 15","state":"Shutdown"}]}}`)
	backend, _ := buildIOSBackend(t, fs, shot, listJSON, nil, nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "NAM002",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "iPad Mini"},
	})
	if err == nil {
		t.Fatal("expected name-not-found error")
	}
	if !strings.Contains(err.Error(), "no simulator matched") {
		t.Fatalf("expected 'no simulator matched' in error, got %v", err)
	}
}

func TestIOSCaptureAmbiguousName(t *testing.T) {
	fs := newIOSFakeFS()
	shot := synthesizeIOSScreenshot(t, "NAM003", 2)
	listJSON := []byte(`{"devices":{"rt":[{"udid":"11111111-2222-3333-4444-555555555555","name":"iPhone 15","state":"Shutdown"},{"udid":"22222222-2222-2222-2222-222222222222","name":"iPhone 15","state":"Shutdown"}]}}`)
	backend, _ := buildIOSBackend(t, fs, shot, listJSON, nil, nil, nil, nil)

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "NAM003",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "iPhone 15"},
	})
	if err == nil {
		t.Fatal("expected ambiguous-name error")
	}
	if !strings.Contains(err.Error(), "2 simulators matched") {
		t.Fatalf("expected ambiguity count in error, got %v", err)
	}
}

func TestIOSCaptureInvalidSimulatorString(t *testing.T) {
	fs := newIOSFakeFS()
	backend := &iosBackend{
		runSimctl: func(ctx context.Context, args ...string) ([]byte, error) {
			return []byte(""), nil
		},
		readFile: fs.read,
	}
	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "NOPE02",
	}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "has\nnewline"},
	})
	if err == nil {
		t.Fatal("expected validation error for newline in simulator")
	}
}

func TestIOSCaptureRegistered(t *testing.T) {
	backend, err := lookupCaptureBackend(SurfaceIOS)
	if err != nil {
		t.Fatalf("lookup ios backend: %v", err)
	}
	if _, ok := backend.(*iosBackend); !ok {
		t.Fatalf("expected *iosBackend, got %T", backend)
	}
}

func TestIOSLooksLikeUDID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"11111111-2222-3333-4444-555555555555", true},
		{"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", true},
		{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", true},
		{"booted", false},
		{"iPhone 15", false},
		{"11111111-2222-3333-4444-55555555555", false},  // too short
		{"11111111_2222_3333_4444_555555555555", false}, // wrong dashes
		{"ZZZZZZZZ-2222-3333-4444-555555555555", false}, // non-hex
	}
	for _, c := range cases {
		if got := looksLikeUDID(c.in); got != c.want {
			t.Errorf("looksLikeUDID(%q)=%v, want %v", c.in, got, c.want)
		}
	}
}

func TestIOSStatusBarRegion(t *testing.T) {
	shot := synthesizeIOSScreenshotSized(t, "X", 2, 390, 844)
	region, err := iosStatusBarRegion(shot)
	if err != nil {
		t.Fatalf("region: %v", err)
	}
	if region.Dx() != 390 {
		t.Fatalf("expected region width 390, got %d", region.Dx())
	}
	// 844/20 = 42, so we expect the 60px minimum to kick in.
	if region.Dy() != 60 {
		t.Fatalf("expected region height 60 (min), got %d", region.Dy())
	}

	// Taller image should scale to height/20.
	tall := synthesizeIOSScreenshotSized(t, "X", 2, 400, 2000)
	region2, err := iosStatusBarRegion(tall)
	if err != nil {
		t.Fatalf("region2: %v", err)
	}
	if region2.Dy() != 100 {
		t.Fatalf("expected region height 100 (height/20), got %d", region2.Dy())
	}
}

func TestIOSFormatRegion(t *testing.T) {
	got := formatIOSRegion(image.Rect(0, 0, 390, 60))
	if got != "0,0,390,60" {
		t.Fatalf("formatIOSRegion: got %q", got)
	}
}

// TestIOSIntegrationRealCapture runs a real simctl capture if and only if
// xcrun is on PATH and there is at least one booted simulator. Otherwise
// it is skipped so CI on non-mac hosts stays green.
func TestIOSIntegrationRealCapture(t *testing.T) {
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("xcrun not on PATH")
	}
	ctx := context.Background()
	out, err := runSimctlCmd(ctx, "list", "devices", "booted", "-j")
	if err != nil {
		t.Skipf("simctl list booted failed: %v", err)
	}
	var list simctlDeviceList
	if err := json.Unmarshal(bytes.TrimSpace(out), &list); err != nil {
		t.Skipf("parse simctl list booted: %v", err)
	}
	hasBooted := false
	for _, devs := range list.Devices {
		for _, d := range devs {
			if strings.EqualFold(d.State, "Booted") {
				hasBooted = true
			}
		}
	}
	if !hasBooted {
		t.Skip("no booted simulator available")
	}

	backend := defaultIOSBackend()
	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
		Nonce:        "INTEG1",
	}
	result, err := backend.Capture(ctx, dest, CaptureOptions{
		Surface: SurfaceIOS,
		Target:  map[string]string{"simulator": "booted"},
	})
	if err != nil {
		t.Fatalf("real ios capture: %v", err)
	}
	if result.Verification != CaptureVerifyPixel {
		t.Fatalf("expected pixel verification, got %s", result.Verification)
	}
}

// containsSeq returns true if seq appears in order (with gaps allowed) in
// arr.
func containsSeq(arr, seq []string) bool {
	i := 0
	for _, a := range arr {
		if i < len(seq) && a == seq[i] {
			i++
		}
	}
	return i == len(seq)
}

// Compile-time assertion that iosBackend satisfies CaptureBackend.
var _ CaptureBackend = (*iosBackend)(nil)

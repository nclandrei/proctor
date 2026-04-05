package proctor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeBrowserPNG returns a byte slice large enough to satisfy
// DefaultMinScreenshotSize so the capture engine does not reject it.
func fakeBrowserPNG() []byte {
	size := int(DefaultMinScreenshotSize) + 1024
	buf := make([]byte, size)
	copy(buf, []byte("\x89PNG\r\n\x1a\n"))
	for i := 8; i < size; i++ {
		buf[i] = byte(i & 0xff)
	}
	return buf
}

// newFakeBrowserBackend constructs a browserBackend whose Chrome invocations
// are mocked. The runChrome fake writes the fixture PNG to the --screenshot
// path so the engine's size/hash checks succeed.
func newFakeBrowserBackend(t *testing.T, content []byte, version string) *browserBackend {
	t.Helper()
	return &browserBackend{
		locateChrome: func() (string, error) {
			return "/fake/chrome", nil
		},
		runChrome: func(ctx context.Context, binary string, args []string) error {
			dest := extractScreenshotArg(args)
			if dest == "" {
				return errors.New("fake chrome: --screenshot flag not set")
			}
			return os.WriteFile(dest, content, 0o644)
		},
		chromeVersion: func(ctx context.Context, binary string) (string, error) {
			return version, nil
		},
	}
}

// extractScreenshotArg pulls the destination path out of a chrome arg list.
func extractScreenshotArg(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, "--screenshot=") {
			return strings.TrimPrefix(a, "--screenshot=")
		}
	}
	return ""
}

// extractArg returns the flag value for the given prefix.
func extractArg(args []string, prefix string) string {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix)
		}
	}
	return ""
}

func TestBrowserCaptureHappyPathDesktop(t *testing.T) {
	content := fakeBrowserPNG()
	backend := newFakeBrowserBackend(t, content, "Google Chrome 120.0.0.0")

	tmp := t.TempDir()
	dest := CaptureDestination{
		ArtifactPath: filepath.Join(tmp, "shot.png"),
		Nonce:        "ABCDEF",
	}
	opts := CaptureOptions{
		Surface: SurfaceBrowser,
		Target: map[string]string{
			"url": "https://example.com/login",
		},
	}
	result, err := backend.Capture(context.Background(), dest, opts)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Verification != CaptureVerifyMeta {
		t.Fatalf("expected verification meta, got %s", result.Verification)
	}
	if result.TranscriptWritten {
		t.Fatal("browser backend should not write transcripts")
	}
	if result.Target["url"] != "https://example.com/login" {
		t.Fatalf("url not recorded: %v", result.Target)
	}
	if result.Target["viewport"] != "desktop" {
		t.Fatalf("expected viewport desktop, got %q", result.Target["viewport"])
	}
	if result.Target["window_size"] != "1280x800" {
		t.Fatalf("expected window_size 1280x800, got %q", result.Target["window_size"])
	}
	if result.Target["chrome_binary"] != "/fake/chrome" {
		t.Fatalf("expected chrome_binary /fake/chrome, got %q", result.Target["chrome_binary"])
	}
	if result.Target["chrome_version"] != "Google Chrome 120.0.0.0" {
		t.Fatalf("expected chrome_version Google Chrome 120.0.0.0, got %q", result.Target["chrome_version"])
	}
	if result.Target["wait_ms"] != "2000" {
		t.Fatalf("expected wait_ms 2000, got %q", result.Target["wait_ms"])
	}

	info, err := os.Stat(dest.ArtifactPath)
	if err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if info.Size() != int64(len(content)) {
		t.Fatalf("expected %d bytes, got %d", len(content), info.Size())
	}
}

func TestBrowserCaptureHappyPathMobile(t *testing.T) {
	backend := newFakeBrowserBackend(t, fakeBrowserPNG(), "Chromium 121.0.0.0")
	tmp := t.TempDir()
	dest := CaptureDestination{
		ArtifactPath: filepath.Join(tmp, "shot.png"),
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceBrowser,
		Target: map[string]string{
			"url":      "https://example.com",
			"viewport": "mobile",
		},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Target["viewport"] != "mobile" {
		t.Fatalf("expected viewport mobile, got %q", result.Target["viewport"])
	}
	if result.Target["window_size"] != "390x844" {
		t.Fatalf("expected window_size 390x844, got %q", result.Target["window_size"])
	}
}

func TestBrowserCaptureHappyPathCustomWindowSize(t *testing.T) {
	var gotArgs []string
	backend := &browserBackend{
		locateChrome: func() (string, error) { return "/fake/chrome", nil },
		runChrome: func(ctx context.Context, binary string, args []string) error {
			gotArgs = args
			dest := extractScreenshotArg(args)
			return os.WriteFile(dest, fakeBrowserPNG(), 0o644)
		},
		chromeVersion: func(ctx context.Context, binary string) (string, error) {
			return "Google Chrome 120.0.0.0", nil
		},
	}
	tmp := t.TempDir()
	result, err := backend.Capture(context.Background(), CaptureDestination{
		ArtifactPath: filepath.Join(tmp, "shot.png"),
	}, CaptureOptions{
		Surface: SurfaceBrowser,
		Target: map[string]string{
			"url":         "https://example.com",
			"viewport":    "desktop",
			"window_size": "1440x900",
			"wait_ms":     "500",
		},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Target["window_size"] != "1440x900" {
		t.Fatalf("expected window_size 1440x900, got %q", result.Target["window_size"])
	}
	if result.Target["wait_ms"] != "500" {
		t.Fatalf("expected wait_ms 500, got %q", result.Target["wait_ms"])
	}
	if got := extractArg(gotArgs, "--window-size="); got != "1440x900" {
		t.Fatalf("expected chrome --window-size=1440x900, got %q", got)
	}
	if got := extractArg(gotArgs, "--virtual-time-budget="); got != "500" {
		t.Fatalf("expected chrome --virtual-time-budget=500, got %q", got)
	}
}

func TestBrowserCaptureChromeArgs(t *testing.T) {
	var gotArgs []string
	backend := &browserBackend{
		locateChrome: func() (string, error) { return "/fake/chrome", nil },
		runChrome: func(ctx context.Context, binary string, args []string) error {
			gotArgs = args
			dest := extractScreenshotArg(args)
			return os.WriteFile(dest, fakeBrowserPNG(), 0o644)
		},
		chromeVersion: func(ctx context.Context, binary string) (string, error) {
			return "Google Chrome 120.0.0.0", nil
		},
	}
	tmp := t.TempDir()
	dest := CaptureDestination{ArtifactPath: filepath.Join(tmp, "shot.png")}
	if _, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceBrowser,
		Target:  map[string]string{"url": "https://example.com"},
	}); err != nil {
		t.Fatalf("capture: %v", err)
	}
	required := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--hide-scrollbars",
		"--force-device-scale-factor=1",
	}
	for _, want := range required {
		found := false
		for _, a := range gotArgs {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing chrome arg %q in %v", want, gotArgs)
		}
	}
	// URL must be the last arg after all flags.
	if gotArgs[len(gotArgs)-1] != "https://example.com" {
		t.Fatalf("expected url as last arg, got %v", gotArgs)
	}
}

func TestBrowserCaptureMissingURL(t *testing.T) {
	backend := newFakeBrowserBackend(t, fakeBrowserPNG(), "Google Chrome 120.0.0.0")
	_, err := backend.Capture(context.Background(), CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
	}, CaptureOptions{Surface: SurfaceBrowser, Target: map[string]string{}})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Fatalf("expected error to mention url, got %v", err)
	}
}

func TestBrowserCaptureInvalidURL(t *testing.T) {
	backend := newFakeBrowserBackend(t, fakeBrowserPNG(), "Google Chrome 120.0.0.0")
	cases := []struct {
		name string
		url  string
	}{
		{"ftp scheme", "ftp://example.com"},
		{"file scheme", "file:///etc/passwd"},
		{"empty scheme", "example.com/path"},
		{"unparseable", "http://%41:8080/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := backend.Capture(context.Background(), CaptureDestination{
				ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
			}, CaptureOptions{Surface: SurfaceBrowser, Target: map[string]string{"url": tc.url}})
			if err == nil {
				t.Fatalf("expected error for url %q", tc.url)
			}
		})
	}
}

func TestBrowserCaptureUnknownViewport(t *testing.T) {
	backend := newFakeBrowserBackend(t, fakeBrowserPNG(), "Google Chrome 120.0.0.0")
	_, err := backend.Capture(context.Background(), CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
	}, CaptureOptions{
		Surface: SurfaceBrowser,
		Target:  map[string]string{"url": "https://example.com", "viewport": "watch"},
	})
	if err == nil {
		t.Fatal("expected error for unknown viewport")
	}
	if !strings.Contains(err.Error(), "viewport") {
		t.Fatalf("expected error to mention viewport, got %v", err)
	}
}

func TestBrowserCaptureNegativeWaitMS(t *testing.T) {
	backend := newFakeBrowserBackend(t, fakeBrowserPNG(), "Google Chrome 120.0.0.0")
	_, err := backend.Capture(context.Background(), CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
	}, CaptureOptions{
		Surface: SurfaceBrowser,
		Target:  map[string]string{"url": "https://example.com", "wait_ms": "-100"},
	})
	if err == nil {
		t.Fatal("expected error for negative wait_ms")
	}
	if !strings.Contains(err.Error(), "wait_ms") {
		t.Fatalf("expected error to mention wait_ms, got %v", err)
	}
}

func TestBrowserCaptureNonIntegerWaitMS(t *testing.T) {
	backend := newFakeBrowserBackend(t, fakeBrowserPNG(), "Google Chrome 120.0.0.0")
	_, err := backend.Capture(context.Background(), CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
	}, CaptureOptions{
		Surface: SurfaceBrowser,
		Target:  map[string]string{"url": "https://example.com", "wait_ms": "soon"},
	})
	if err == nil {
		t.Fatal("expected error for non-integer wait_ms")
	}
}

func TestBrowserCaptureInvalidWindowSize(t *testing.T) {
	backend := newFakeBrowserBackend(t, fakeBrowserPNG(), "Google Chrome 120.0.0.0")
	cases := []string{"", "1280", "abcxdef", "1280x", "x800", "-1x100", "0x0"}
	for _, ws := range cases {
		if ws == "" {
			continue // empty falls back to viewport preset
		}
		t.Run(ws, func(t *testing.T) {
			_, err := backend.Capture(context.Background(), CaptureDestination{
				ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
			}, CaptureOptions{
				Surface: SurfaceBrowser,
				Target:  map[string]string{"url": "https://example.com", "window_size": ws},
			})
			if err == nil {
				t.Fatalf("expected error for window_size %q", ws)
			}
		})
	}
}

func TestBrowserCaptureChromeNotFound(t *testing.T) {
	backend := &browserBackend{
		locateChrome: func() (string, error) {
			return "", errors.New("browser capture: no Chrome binary found (set PROCTOR_CHROME or install Google Chrome / Chromium)")
		},
		runChrome:     func(ctx context.Context, binary string, args []string) error { return nil },
		chromeVersion: func(ctx context.Context, binary string) (string, error) { return "", nil },
	}
	_, err := backend.Capture(context.Background(), CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
	}, CaptureOptions{
		Surface: SurfaceBrowser,
		Target:  map[string]string{"url": "https://example.com"},
	})
	if err == nil {
		t.Fatal("expected error when chrome not found")
	}
	if !strings.Contains(err.Error(), "PROCTOR_CHROME") {
		t.Fatalf("expected error to mention PROCTOR_CHROME, got %v", err)
	}
}

func TestBrowserCaptureChromeExecFailure(t *testing.T) {
	backend := &browserBackend{
		locateChrome: func() (string, error) { return "/fake/chrome", nil },
		runChrome: func(ctx context.Context, binary string, args []string) error {
			return errors.New("exit status 137")
		},
		chromeVersion: func(ctx context.Context, binary string) (string, error) {
			return "Google Chrome 120.0.0.0", nil
		},
	}
	_, err := backend.Capture(context.Background(), CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "shot.png"),
	}, CaptureOptions{
		Surface: SurfaceBrowser,
		Target:  map[string]string{"url": "https://example.com"},
	})
	if err == nil {
		t.Fatal("expected error when chrome exec fails")
	}
	if !strings.Contains(err.Error(), "chrome exec failed") {
		t.Fatalf("expected wrapped chrome exec failed error, got %v", err)
	}
	if !strings.Contains(err.Error(), "exit status 137") {
		t.Fatalf("expected underlying error preserved, got %v", err)
	}
}

func TestLocateChromeBinaryOverride(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "my-chrome")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROCTOR_CHROME", fake)
	got, err := locateChromeBinary()
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != fake {
		t.Fatalf("expected %s, got %s", fake, got)
	}
}

func TestLocateChromeBinaryOverrideMissing(t *testing.T) {
	t.Setenv("PROCTOR_CHROME", "/does/not/exist/chrome")
	_, err := locateChromeBinary()
	if err == nil {
		t.Fatal("expected error for missing PROCTOR_CHROME path")
	}
}

func TestValidateWindowSize(t *testing.T) {
	good := []string{"1280x800", "390x844", "1x1", "3840x2160"}
	for _, ws := range good {
		if err := validateWindowSize(ws); err != nil {
			t.Fatalf("expected %q ok, got %v", ws, err)
		}
	}
	bad := []string{"", "1280", "abcxdef", "1280x", "x800", "-1x100", "0x0", "1280x0", "0x800"}
	for _, ws := range bad {
		if err := validateWindowSize(ws); err == nil {
			t.Fatalf("expected %q invalid", ws)
		}
	}
}

func TestBrowserBackendIsRegistered(t *testing.T) {
	backend, err := lookupCaptureBackend(SurfaceBrowser)
	if err != nil {
		t.Fatalf("lookup browser backend: %v", err)
	}
	if _, ok := backend.(*browserBackend); !ok {
		t.Fatalf("expected *browserBackend, got %T", backend)
	}
}

func TestBrowserCaptureIntegration(t *testing.T) {
	if _, err := locateChromeBinary(); err != nil {
		t.Skipf("no chrome binary available; skipping integration test (%v)", err)
	}
	backend := defaultBrowserBackend()
	tmp := t.TempDir()
	dest := CaptureDestination{ArtifactPath: filepath.Join(tmp, "integration.png")}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceBrowser,
		Target: map[string]string{
			"url":      "https://example.com",
			"viewport": "desktop",
			"wait_ms":  strconv.Itoa(2000),
		},
	})
	if err != nil {
		t.Fatalf("integration capture: %v", err)
	}
	info, err := os.Stat(dest.ArtifactPath)
	if err != nil {
		t.Fatalf("artifact not written: %v", err)
	}
	if info.Size() < DefaultMinScreenshotSize {
		t.Fatalf("artifact too small: %d bytes (min %d)", info.Size(), DefaultMinScreenshotSize)
	}
	if result.Target["chrome_binary"] == "" {
		t.Fatal("chrome_binary not recorded")
	}
	// Verify this really looks like a PNG.
	data, err := os.ReadFile(dest.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("artifact does not have PNG magic bytes; got %q", data[:min(8, len(data))])
	}
	if result.Verification != CaptureVerifyMeta {
		t.Fatalf("expected verification meta, got %s", result.Verification)
	}
}

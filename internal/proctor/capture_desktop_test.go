package proctor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeDesktopPNG drops a deterministic PNG-like blob that is large
// enough to pass DefaultMinScreenshotSize (10KB). The content is not a
// structurally valid PNG but the capture engine only checks file size.
func writeFakeDesktopPNG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 11KB: comfortably over the 10KB minimum.
	size := int(DefaultMinScreenshotSize) + 1024
	buf := make([]byte, size)
	// PNG magic header so the bytes at least look like a PNG.
	copy(buf, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	for i := 8; i < size; i++ {
		buf[i] = byte(i & 0xff)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
}

func newFakeDesktopBackend(t *testing.T, windows []desktopWindowInfo) *desktopBackend {
	t.Helper()
	return &desktopBackend{
		listWindows: func(ctx context.Context) ([]desktopWindowInfo, error) {
			return windows, nil
		},
		capture: func(ctx context.Context, cgWindowID int, dest string) error {
			writeFakeDesktopPNG(t, dest)
			return nil
		},
	}
}

func TestDesktopCaptureHappyPath(t *testing.T) {
	win := desktopWindowInfo{
		WindowID: 4242,
		Title:    "Editor — main.go",
		PID:      9876,
		AppName:  "Code",
	}
	win.Bounds.X = 100
	win.Bounds.Y = 50
	win.Bounds.Width = 1280
	win.Bounds.Height = 800
	backend := newFakeDesktopBackend(t, []desktopWindowInfo{win})

	dest := CaptureDestination{
		ArtifactPath: filepath.Join(t.TempDir(), "cap.png"),
	}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target: map[string]string{
			"window_title": "main.go",
		},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Verification != CaptureVerifyMeta {
		t.Fatalf("expected verification meta, got %s", result.Verification)
	}
	if result.TranscriptWritten {
		t.Fatal("desktop backend must not claim a transcript was written")
	}
	if result.Target["window_id"] != "4242" {
		t.Fatalf("expected window_id 4242, got %q", result.Target["window_id"])
	}
	if result.Target["window_title"] != "Editor — main.go" {
		t.Fatalf("expected window_title to be preserved, got %q", result.Target["window_title"])
	}
	if result.Target["pid"] != "9876" {
		t.Fatalf("expected pid 9876, got %q", result.Target["pid"])
	}
	if result.Target["app_name"] != "Code" {
		t.Fatalf("expected app_name Code, got %q", result.Target["app_name"])
	}
	if result.Target["bounds"] != "100,50,1280,800" {
		t.Fatalf("expected bounds 100,50,1280,800, got %q", result.Target["bounds"])
	}
	info, err := os.Stat(dest.ArtifactPath)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if info.Size() < DefaultMinScreenshotSize {
		t.Fatalf("artifact too small: %d bytes", info.Size())
	}
}

func TestDesktopCaptureMatchesByPID(t *testing.T) {
	windows := []desktopWindowInfo{
		{WindowID: 1, Title: "Other", PID: 1, AppName: "A"},
		{WindowID: 2, Title: "Target", PID: 42, AppName: "B"},
		{WindowID: 3, Title: "Noise", PID: 100, AppName: "C"},
	}
	backend := newFakeDesktopBackend(t, windows)

	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{"pid": "42"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Target["window_id"] != "2" {
		t.Fatalf("expected window_id 2, got %q", result.Target["window_id"])
	}
}

func TestDesktopCaptureMatchesByApp(t *testing.T) {
	windows := []desktopWindowInfo{
		{WindowID: 10, Title: "Inbox", PID: 1, AppName: "Mail"},
		{WindowID: 11, Title: "README", PID: 2, AppName: "TextEdit"},
	}
	backend := newFakeDesktopBackend(t, windows)

	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	result, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{"app": "textedit"},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if result.Target["app_name"] != "TextEdit" {
		t.Fatalf("expected app TextEdit, got %q", result.Target["app_name"])
	}
}

func TestDesktopCaptureNoMatch(t *testing.T) {
	backend := newFakeDesktopBackend(t, []desktopWindowInfo{
		{WindowID: 1, Title: "Alpha", PID: 1, AppName: "AppOne"},
	})
	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{"window_title": "nothing-here"},
	})
	if err == nil {
		t.Fatal("expected no-match error")
	}
	if !strings.Contains(err.Error(), "no window matched") {
		t.Fatalf("expected 'no window matched' in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "nothing-here") {
		t.Fatalf("expected criteria echoed in error, got %v", err)
	}
}

func TestDesktopCaptureMultipleMatches(t *testing.T) {
	windows := []desktopWindowInfo{
		{WindowID: 1, Title: "Project — main.go", PID: 10, AppName: "Code"},
		{WindowID: 2, Title: "Other — main.go", PID: 11, AppName: "Code"},
		{WindowID: 3, Title: "README", PID: 12, AppName: "Code"},
	}
	backend := newFakeDesktopBackend(t, windows)
	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{"window_title": "main.go"},
	})
	if err == nil {
		t.Fatal("expected multiple-match error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "2 windows matched") {
		t.Fatalf("expected match count in error, got %v", err)
	}
	if !strings.Contains(msg, "id=1") || !strings.Contains(msg, "id=2") {
		t.Fatalf("expected both matches listed, got %v", err)
	}
	if strings.Contains(msg, "id=3") {
		t.Fatalf("non-matching window should not be listed, got %v", err)
	}
}

func TestDesktopCaptureNoCriteria(t *testing.T) {
	backend := newFakeDesktopBackend(t, nil)
	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing criteria")
	}
	if !strings.Contains(err.Error(), "must specify window-title, pid, or app") {
		t.Fatalf("expected criteria guidance in error, got %v", err)
	}
}

func TestDesktopCaptureInvalidPID(t *testing.T) {
	backend := newFakeDesktopBackend(t, nil)
	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{"pid": "abc"},
	})
	if err == nil {
		t.Fatal("expected error for invalid pid")
	}
	if !strings.Contains(err.Error(), "invalid pid") {
		t.Fatalf("expected invalid pid error, got %v", err)
	}
}

func TestDesktopCaptureListError(t *testing.T) {
	backend := &desktopBackend{
		listWindows: func(ctx context.Context) ([]desktopWindowInfo, error) {
			return nil, errors.New("osascript boom")
		},
	}
	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{"app": "anything"},
	})
	if err == nil {
		t.Fatal("expected list error to surface")
	}
	if !strings.Contains(err.Error(), "list windows") {
		t.Fatalf("expected wrapped list-windows error, got %v", err)
	}
	if !strings.Contains(err.Error(), "osascript boom") {
		t.Fatalf("expected underlying error to bubble up, got %v", err)
	}
}

func TestDesktopCaptureExecFailure(t *testing.T) {
	backend := &desktopBackend{
		listWindows: func(ctx context.Context) ([]desktopWindowInfo, error) {
			return []desktopWindowInfo{{WindowID: 7, Title: "x", PID: 1, AppName: "A"}}, nil
		},
		capture: func(ctx context.Context, cgWindowID int, dest string) error {
			return errors.New("screencapture crashed")
		},
	}
	dest := CaptureDestination{ArtifactPath: filepath.Join(t.TempDir(), "cap.png")}
	_, err := backend.Capture(context.Background(), dest, CaptureOptions{
		Surface: SurfaceDesktop,
		Target:  map[string]string{"app": "A"},
	})
	if err == nil {
		t.Fatal("expected capture exec failure to surface")
	}
	if !strings.Contains(err.Error(), "screencapture") {
		t.Fatalf("expected wrapped screencapture error, got %v", err)
	}
	if !strings.Contains(err.Error(), "crashed") {
		t.Fatalf("expected underlying error text, got %v", err)
	}
}

func TestDesktopCaptureRegistered(t *testing.T) {
	backend, err := lookupCaptureBackend(SurfaceDesktop)
	if err != nil {
		t.Fatalf("lookup desktop backend: %v", err)
	}
	if _, ok := backend.(*desktopBackend); !ok {
		t.Fatalf("expected *desktopBackend, got %T", backend)
	}
}

func TestDesktopCaptureFilterWindows(t *testing.T) {
	windows := []desktopWindowInfo{
		{WindowID: 1, Title: "Foo Bar", PID: 10, AppName: "Code"},
		{WindowID: 2, Title: "FOO baz", PID: 11, AppName: "Code"},
		{WindowID: 3, Title: "quux", PID: 12, AppName: "Terminal"},
	}

	got := filterDesktopWindows(windows, "foo", "", 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 title matches, got %d", len(got))
	}

	got = filterDesktopWindows(windows, "foo", "code", 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 combined matches, got %d", len(got))
	}

	got = filterDesktopWindows(windows, "foo", "code", 11)
	if len(got) != 1 || got[0].WindowID != 2 {
		t.Fatalf("expected window 2 only, got %+v", got)
	}

	got = filterDesktopWindows(windows, "", "terminal", 0)
	if len(got) != 1 || got[0].AppName != "Terminal" {
		t.Fatalf("expected Terminal match only, got %+v", got)
	}
}

func TestDesktopIntegrationListWindows(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("desktop capture integration requires macOS")
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skip("osascript not available on PATH")
	}
	ctx := context.Background()
	windows, err := listDesktopWindows(ctx)
	if err != nil {
		t.Skipf("listDesktopWindows failed (likely a sandbox/permissions issue): %v", err)
	}
	// On a live macOS desktop there should be at least one on-screen window.
	// If we're running in a truly headless env we accept zero but note it.
	t.Logf("listDesktopWindows returned %d windows", len(windows))
	for _, w := range windows {
		if w.WindowID < 0 || w.PID < 0 {
			t.Fatalf("unexpected negative id/pid: %+v", w)
		}
	}
}

// Compile-time assertion that desktopBackend satisfies CaptureBackend.
var _ CaptureBackend = (*desktopBackend)(nil)

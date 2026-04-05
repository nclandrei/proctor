package proctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// desktopWindowInfo describes one on-screen window as reported by the macOS
// Quartz window server. It is the seam between the real osascript-backed
// discovery path and the fake injected by tests.
type desktopWindowInfo struct {
	WindowID int    `json:"windowId"`
	Title    string `json:"title"`
	PID      int    `json:"pid"`
	AppName  string `json:"appName"`
	Bounds   struct {
		X      float64 `json:"X"`
		Y      float64 `json:"Y"`
		Width  float64 `json:"Width"`
		Height float64 `json:"Height"`
	} `json:"bounds"`
}

// desktopBackend is the capture backend for the desktop surface. On macOS it
// uses osascript (JXA) to ask Quartz for on-screen window info and the
// screencapture binary to grab a PNG of the matched window. Both seams are
// replaced in tests.
type desktopBackend struct {
	listWindows func(ctx context.Context) ([]desktopWindowInfo, error)
	capture     func(ctx context.Context, cgWindowID int, dest string) error
}

// Capture finds exactly one window matching the caller's criteria and writes a
// PNG of that window to dest.ArtifactPath. The resulting Target carries
// window_id, window_title, pid, app_name, and bounds so report consumers can
// cross-check the capture against the scenario.
func (b *desktopBackend) Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error) {
	wantTitle := strings.TrimSpace(opts.Target["window_title"])
	wantApp := strings.TrimSpace(opts.Target["app"])
	wantPIDStr := strings.TrimSpace(opts.Target["pid"])

	if wantTitle == "" && wantApp == "" && wantPIDStr == "" {
		return CaptureResult{}, fmt.Errorf("desktop capture: must specify window-title, pid, or app")
	}

	wantPID := 0
	if wantPIDStr != "" {
		parsed, err := strconv.Atoi(wantPIDStr)
		if err != nil || parsed <= 0 {
			return CaptureResult{}, fmt.Errorf("desktop capture: invalid pid %q", wantPIDStr)
		}
		wantPID = parsed
	}

	list := b.listWindows
	if list == nil {
		list = listDesktopWindows
	}
	windows, err := list(ctx)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("desktop capture: list windows: %w", err)
	}

	matches := filterDesktopWindows(windows, wantTitle, wantApp, wantPID)
	if len(matches) == 0 {
		return CaptureResult{}, fmt.Errorf("desktop capture: no window matched %s", describeDesktopCriteria(wantTitle, wantApp, wantPID))
	}
	if len(matches) > 1 {
		return CaptureResult{}, fmt.Errorf("desktop capture: %d windows matched %s; %s", len(matches), describeDesktopCriteria(wantTitle, wantApp, wantPID), summarizeDesktopMatches(matches))
	}

	match := matches[0]

	grab := b.capture
	if grab == nil {
		grab = runScreencapture
	}
	if err := grab(ctx, match.WindowID, dest.ArtifactPath); err != nil {
		return CaptureResult{}, fmt.Errorf("desktop capture: screencapture: %w", err)
	}

	target := map[string]string{
		"window_id":    strconv.Itoa(match.WindowID),
		"window_title": match.Title,
		"pid":          strconv.Itoa(match.PID),
		"app_name":     match.AppName,
		"bounds":       formatDesktopBounds(match),
	}

	return CaptureResult{
		Target:       target,
		Verification: CaptureVerifyMeta,
	}, nil
}

// filterDesktopWindows returns every window that satisfies all the supplied
// criteria. Empty criteria are ignored. Title matching is case-insensitive
// contains.
func filterDesktopWindows(windows []desktopWindowInfo, wantTitle, wantApp string, wantPID int) []desktopWindowInfo {
	lowerTitle := strings.ToLower(wantTitle)
	lowerApp := strings.ToLower(wantApp)
	var out []desktopWindowInfo
	for _, w := range windows {
		if wantPID > 0 && w.PID != wantPID {
			continue
		}
		if lowerApp != "" && !strings.Contains(strings.ToLower(w.AppName), lowerApp) {
			continue
		}
		if lowerTitle != "" && !strings.Contains(strings.ToLower(w.Title), lowerTitle) {
			continue
		}
		out = append(out, w)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].WindowID < out[j].WindowID })
	return out
}

func describeDesktopCriteria(wantTitle, wantApp string, wantPID int) string {
	var parts []string
	if wantTitle != "" {
		parts = append(parts, fmt.Sprintf("window-title=%q", wantTitle))
	}
	if wantApp != "" {
		parts = append(parts, fmt.Sprintf("app=%q", wantApp))
	}
	if wantPID > 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", wantPID))
	}
	if len(parts) == 0 {
		return "(no criteria)"
	}
	return strings.Join(parts, ", ")
}

func summarizeDesktopMatches(matches []desktopWindowInfo) string {
	if len(matches) == 0 {
		return "no matches"
	}
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, fmt.Sprintf("[id=%d pid=%d app=%q title=%q]", m.WindowID, m.PID, m.AppName, m.Title))
	}
	return "matches: " + strings.Join(parts, " ")
}

func formatDesktopBounds(w desktopWindowInfo) string {
	return fmt.Sprintf("%g,%g,%g,%g", w.Bounds.X, w.Bounds.Y, w.Bounds.Width, w.Bounds.Height)
}

// desktopListWindowsScript is the JavaScript-for-Automation payload that asks
// Quartz for the on-screen window list and emits it as JSON. It is executed
// via `osascript -l JavaScript`.
const desktopListWindowsScript = `
ObjC.import('CoreGraphics');
ObjC.import('Foundation');
var opts = $.kCGWindowListOptionOnScreenOnly | $.kCGWindowListExcludeDesktopElements;
var info = $.CGWindowListCopyWindowInfo(opts, $.kCGNullWindowID);
var arr = ObjC.deepUnwrap(info);
var out = [];
for (var i = 0; i < arr.length; i++) {
  var w = arr[i];
  var bounds = w['kCGWindowBounds'] || {};
  out.push({
    windowId: w['kCGWindowNumber'] || 0,
    title: w['kCGWindowName'] || '',
    pid: w['kCGWindowOwnerPID'] || 0,
    appName: w['kCGWindowOwnerName'] || '',
    bounds: {
      X: bounds['X'] || 0,
      Y: bounds['Y'] || 0,
      Width: bounds['Width'] || 0,
      Height: bounds['Height'] || 0,
    },
  });
}
JSON.stringify(out);
`

// listDesktopWindows is the real-world window enumerator. It runs the JXA
// script via osascript and parses the JSON payload.
func listDesktopWindows(ctx context.Context) ([]desktopWindowInfo, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("desktop capture is only supported on macOS (current GOOS=%s)", runtime.GOOS)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", desktopListWindowsScript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("osascript: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return nil, nil
	}
	var windows []desktopWindowInfo
	if err := json.Unmarshal(raw, &windows); err != nil {
		return nil, fmt.Errorf("parse osascript output: %w", err)
	}
	return windows, nil
}

// runScreencapture is the real-world screenshot path. It invokes
// `screencapture -o -x -l<cgWindowID> <dest>` which captures the window
// without shadow, without sound, by its Quartz window id.
func runScreencapture(ctx context.Context, cgWindowID int, dest string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("desktop capture is only supported on macOS (current GOOS=%s)", runtime.GOOS)
	}
	cmd := exec.CommandContext(ctx, "/usr/sbin/screencapture", "-o", "-x", fmt.Sprintf("-l%d", cgWindowID), dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func init() {
	RegisterCaptureBackend(SurfaceDesktop, &desktopBackend{})
}

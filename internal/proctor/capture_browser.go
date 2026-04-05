package proctor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// browserBackend captures web screenshots by shelling out to Chrome in
// headless mode. Because there is no CDP WebSocket client yet, the browser
// MVP relies on --screenshot and metadata verification (CaptureVerifyMeta)
// rather than pixel-level nonce matching.
type browserBackend struct {
	locateChrome  func() (string, error)
	runChrome     func(ctx context.Context, binary string, args []string) error
	chromeVersion func(ctx context.Context, binary string) (string, error)
}

// defaultBrowserBackend constructs a backend that uses the real Chrome
// binary via exec.CommandContext.
func defaultBrowserBackend() *browserBackend {
	return &browserBackend{
		locateChrome:  locateChromeBinary,
		runChrome:     runChromeCmd,
		chromeVersion: readChromeVersion,
	}
}

func init() {
	RegisterCaptureBackend(SurfaceBrowser, defaultBrowserBackend())
}

// knownViewports maps viewport names to their WxH dimensions.
var knownViewports = map[string]string{
	"desktop": "1280x800",
	"mobile":  "390x844",
}

const defaultBrowserWaitMS = 2000

// Capture resolves the Chrome binary, renders the target URL to PNG, and
// records target metadata that can be cross-checked on verification.
func (b *browserBackend) Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error) {
	rawURL := strings.TrimSpace(opts.Target["url"])
	if rawURL == "" {
		return CaptureResult{}, errors.New("browser capture requires target url")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("browser capture: parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return CaptureResult{}, fmt.Errorf("browser capture: url must be http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return CaptureResult{}, fmt.Errorf("browser capture: url is missing host: %s", rawURL)
	}

	viewport := strings.TrimSpace(opts.Target["viewport"])
	if viewport == "" {
		viewport = "desktop"
	}
	if _, ok := knownViewports[viewport]; !ok {
		return CaptureResult{}, fmt.Errorf("browser capture: unknown viewport %q (want desktop or mobile)", viewport)
	}

	windowSize := strings.TrimSpace(opts.Target["window_size"])
	if windowSize == "" {
		windowSize = knownViewports[viewport]
	} else {
		if err := validateWindowSize(windowSize); err != nil {
			return CaptureResult{}, fmt.Errorf("browser capture: %w", err)
		}
	}

	waitMS := defaultBrowserWaitMS
	if raw := strings.TrimSpace(opts.Target["wait_ms"]); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return CaptureResult{}, fmt.Errorf("browser capture: wait_ms must be an integer, got %q", raw)
		}
		if n < 0 {
			return CaptureResult{}, fmt.Errorf("browser capture: wait_ms must be non-negative, got %d", n)
		}
		waitMS = n
	}

	binary, err := b.locateChrome()
	if err != nil {
		return CaptureResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(dest.ArtifactPath), 0o755); err != nil {
		return CaptureResult{}, fmt.Errorf("browser capture: prepare artifact dir: %w", err)
	}

	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--hide-scrollbars",
		"--force-device-scale-factor=1",
		"--screenshot=" + dest.ArtifactPath,
		"--window-size=" + windowSize,
		"--virtual-time-budget=" + strconv.Itoa(waitMS),
		rawURL,
	}
	if err := b.runChrome(ctx, binary, args); err != nil {
		return CaptureResult{}, fmt.Errorf("browser capture: chrome exec failed: %w", err)
	}

	version := ""
	if b.chromeVersion != nil {
		if v, err := b.chromeVersion(ctx, binary); err == nil {
			version = strings.TrimSpace(v)
		}
	}

	target := map[string]string{
		"url":           rawURL,
		"viewport":      viewport,
		"window_size":   windowSize,
		"chrome_binary": binary,
		"wait_ms":       strconv.Itoa(waitMS),
	}
	if version != "" {
		target["chrome_version"] = version
	}

	return CaptureResult{
		Target:       target,
		Verification: CaptureVerifyMeta,
	}, nil
}

// validateWindowSize checks that the provided value parses as WxH with
// positive integers on both sides.
func validateWindowSize(ws string) error {
	parts := strings.SplitN(ws, "x", 2)
	if len(parts) != 2 {
		return fmt.Errorf("window_size must look like WxH, got %q", ws)
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil || w <= 0 {
		return fmt.Errorf("window_size width must be a positive integer, got %q", parts[0])
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil || h <= 0 {
		return fmt.Errorf("window_size height must be a positive integer, got %q", parts[1])
	}
	return nil
}

// chromeCandidates is the ordered list of paths to try when PROCTOR_CHROME
// is unset. macOS paths are checked explicitly; the remaining names fall
// back to exec.LookPath.
var chromeCandidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
}

var chromePathNames = []string{
	"google-chrome-stable",
	"google-chrome",
	"chromium",
	"chromium-browser",
}

// locateChromeBinary resolves the Chrome binary to use for captures.
func locateChromeBinary() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PROCTOR_CHROME")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("PROCTOR_CHROME=%s: %w", override, err)
		}
		return override, nil
	}
	for _, path := range chromeCandidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	for _, name := range chromePathNames {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("browser capture: no Chrome binary found (set PROCTOR_CHROME or install Google Chrome / Chromium)")
}

// runChromeCmd runs Chrome with the given args, returning any launch error.
func runChromeCmd(ctx context.Context, binary string, args []string) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return fmt.Errorf("%w: %s", err, trimmed)
		}
		return err
	}
	return nil
}

// readChromeVersion shells out to `<chrome> --version` and returns the
// resulting string (e.g., "Google Chrome 120.0.0.0").
func readChromeVersion(ctx context.Context, binary string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

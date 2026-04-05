package proctor

// This file implements the iOS simulator capture backend. The filename
// intentionally carries a _backend suffix instead of plain capture_ios.go
// because Go treats files ending in _ios.go as an implicit GOOS=ios
// build constraint. The iOS simulator backend runs on the developer's
// Mac, not on iOS devices, so we must avoid that collision. The
// capture_stubs.go init() is defensive: it only registers its stub when
// the surface has no backend yet, so registration order is safe either
// way.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"strings"
	"time"
)

// iosBackend captures iPhone/iPad simulator screenshots by shelling out to
// `xcrun simctl`. It plants the proctor nonce into the simulator's status
// bar via `simctl status_bar override --operatorName` so the captured PNG
// can be pixel-verified after the fact. Both exec seams are replaceable in
// tests.
type iosBackend struct {
	runSimctl func(ctx context.Context, args ...string) ([]byte, error)
	readFile  func(path string) ([]byte, error)
}

// defaultIOSBackend constructs an iosBackend that uses the real xcrun
// binary and os.ReadFile.
func defaultIOSBackend() *iosBackend {
	return &iosBackend{
		runSimctl: runSimctlCmd,
		readFile:  os.ReadFile,
	}
}

func init() {
	RegisterCaptureBackend(SurfaceIOS, defaultIOSBackend())
}

// iosLaunchSettleDuration is the short sleep after `simctl launch` so the
// app has time to raise its window before we plant the nonce in the status
// bar and capture.
const iosLaunchSettleDuration = 200 * time.Millisecond

// iosNonceTolerance is the template-match tolerance for the status bar
// nonce. 0.85 matches what browser-side pixel verification uses.
const iosNonceTolerance = 0.85

// iosStatusBarMinHeight is the floor for the status bar scan region when
// the captured image is unusually short.
const iosStatusBarMinHeight = 60

// simctlDeviceList is the JSON envelope returned by `xcrun simctl list
// devices -j`. Only the fields we consume are modeled; everything else is
// ignored by json.Unmarshal.
type simctlDeviceList struct {
	Devices map[string][]simctlDevice `json:"devices"`
}

type simctlDevice struct {
	UDID  string `json:"udid"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Capture plants the nonce in the simulator status bar, shoots a PNG via
// `simctl io ... screenshot`, pixel-verifies the nonce in the top strip of
// the capture, and clears the status bar override on the way out. The
// clear runs in a defer so it fires even when launch or capture fails.
func (b *iosBackend) Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error) {
	simulator := strings.TrimSpace(opts.Target["simulator"])
	if simulator == "" {
		simulator = "booted"
	}
	if err := validateSimulatorIdentifier(simulator); err != nil {
		return CaptureResult{}, err
	}

	bundleID := strings.TrimSpace(opts.Target["bundle_id"])

	runSimctl := b.runSimctl
	if runSimctl == nil {
		runSimctl = runSimctlCmd
	}
	readFile := b.readFile
	if readFile == nil {
		readFile = os.ReadFile
	}

	udid, deviceName, err := resolveSimulatorUDID(ctx, runSimctl, simulator)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("ios capture: resolve simulator: %w", err)
	}

	if bundleID != "" {
		if _, err := runSimctl(ctx, "launch", udid, bundleID); err != nil {
			return CaptureResult{}, fmt.Errorf("ios capture: launch %s: %w", bundleID, err)
		}
		// Give the app a moment to become foregrounded before we touch
		// the status bar so our override is not stomped on by the app's
		// own status-bar style changes.
		time.Sleep(iosLaunchSettleDuration)
	}

	operatorName := "proctor:" + dest.Nonce
	if _, err := runSimctl(ctx, "status_bar", udid, "override", "--operatorName", operatorName); err != nil {
		return CaptureResult{}, fmt.Errorf("ios capture: plant status bar nonce: %w", err)
	}
	// Clear the override even if capture or verification fails. We swallow
	// the clear error: the capture itself is authoritative and a stale
	// override on an ephemeral simulator is not worth failing a run over.
	defer func() {
		_, _ = runSimctl(ctx, "status_bar", udid, "clear")
	}()

	if _, err := runSimctl(ctx, "io", udid, "screenshot", dest.ArtifactPath); err != nil {
		return CaptureResult{}, fmt.Errorf("ios capture: screenshot: %w", err)
	}

	pngBytes, err := readFile(dest.ArtifactPath)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("ios capture: read screenshot: %w", err)
	}

	region, err := iosStatusBarRegion(pngBytes)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("ios capture: %w", err)
	}

	matchedScale, verr := verifyIOSNonce(pngBytes, dest.Nonce, region)
	if verr != nil {
		return CaptureResult{}, fmt.Errorf("ios capture: pixel verification failed: %w", verr)
	}

	target := map[string]string{
		"simulator":    udid,
		"nonce_region": formatIOSRegion(region),
		"scale":        fmt.Sprintf("%d", matchedScale),
	}
	if deviceName != "" {
		target["simulator_name"] = deviceName
	}
	if bundleID != "" {
		target["bundle_id"] = bundleID
	}

	return CaptureResult{
		Target:       target,
		Verification: CaptureVerifyPixel,
	}, nil
}

// validateSimulatorIdentifier rejects obviously malformed --simulator
// values. We accept "booted" literal, any UDID (36-char hex-with-dashes),
// or any human-readable device name that doesn't contain shell-hostile
// characters.
func validateSimulatorIdentifier(id string) error {
	if id == "" {
		return fmt.Errorf("ios capture: simulator identifier is empty")
	}
	if id == "booted" {
		return nil
	}
	if looksLikeUDID(id) {
		return nil
	}
	// Treat anything else as a device name. Disallow control chars and
	// nul bytes so we don't feed something bizarre to xcrun.
	for _, r := range id {
		if r == 0 || r == '\n' || r == '\r' {
			return fmt.Errorf("ios capture: simulator name contains invalid characters: %q", id)
		}
	}
	return nil
}

// looksLikeUDID reports whether s has the shape of an Apple simulator
// UDID: 36 characters with dashes at positions 8, 13, 18, 23 and
// hex/uppercase characters elsewhere.
func looksLikeUDID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHexRune(r) {
				return false
			}
		}
	}
	return true
}

func isHexRune(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'f':
		return true
	case r >= 'A' && r <= 'F':
		return true
	}
	return false
}

// resolveSimulatorUDID turns the caller-supplied simulator identifier into
// a concrete UDID. If the identifier is "booted" or is already a UDID we
// return it directly; otherwise we fall back to `simctl list devices -j`
// and look for a device whose name matches. If multiple devices share the
// name we prefer a booted one and otherwise return an error so ambiguity
// does not silently pick the wrong simulator.
func resolveSimulatorUDID(ctx context.Context, runSimctl func(ctx context.Context, args ...string) ([]byte, error), id string) (string, string, error) {
	if id == "booted" {
		name, _ := lookupBootedDeviceName(ctx, runSimctl)
		return "booted", name, nil
	}
	if looksLikeUDID(id) {
		name, _ := lookupDeviceNameByUDID(ctx, runSimctl, id)
		return id, name, nil
	}
	// Treat id as a device name.
	out, err := runSimctl(ctx, "list", "devices", "-j")
	if err != nil {
		return "", "", fmt.Errorf("list devices: %w", err)
	}
	var list simctlDeviceList
	if err := json.Unmarshal(bytes.TrimSpace(out), &list); err != nil {
		return "", "", fmt.Errorf("parse simctl list: %w", err)
	}
	var candidates []simctlDevice
	for _, devs := range list.Devices {
		for _, d := range devs {
			if strings.EqualFold(d.Name, id) {
				candidates = append(candidates, d)
			}
		}
	}
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("no simulator matched name %q", id)
	}
	// Prefer a booted simulator if we have a name collision.
	for _, c := range candidates {
		if strings.EqualFold(c.State, "Booted") {
			return c.UDID, c.Name, nil
		}
	}
	if len(candidates) > 1 {
		return "", "", fmt.Errorf("%d simulators matched name %q; specify a UDID", len(candidates), id)
	}
	return candidates[0].UDID, candidates[0].Name, nil
}

// lookupBootedDeviceName is best-effort metadata enrichment: if simctl is
// happy to tell us which device is booted, we record it in the target map
// so reports show something more human-friendly than "booted".
func lookupBootedDeviceName(ctx context.Context, runSimctl func(ctx context.Context, args ...string) ([]byte, error)) (string, error) {
	out, err := runSimctl(ctx, "list", "devices", "booted", "-j")
	if err != nil {
		return "", err
	}
	var list simctlDeviceList
	if err := json.Unmarshal(bytes.TrimSpace(out), &list); err != nil {
		return "", err
	}
	for _, devs := range list.Devices {
		for _, d := range devs {
			if strings.EqualFold(d.State, "Booted") {
				return d.Name, nil
			}
		}
	}
	return "", nil
}

// lookupDeviceNameByUDID does the reverse mapping: given a UDID, find its
// name. Errors are swallowed by callers because the UDID itself is always
// sufficient to perform the capture.
func lookupDeviceNameByUDID(ctx context.Context, runSimctl func(ctx context.Context, args ...string) ([]byte, error), udid string) (string, error) {
	out, err := runSimctl(ctx, "list", "devices", "-j")
	if err != nil {
		return "", err
	}
	var list simctlDeviceList
	if err := json.Unmarshal(bytes.TrimSpace(out), &list); err != nil {
		return "", err
	}
	for _, devs := range list.Devices {
		for _, d := range devs {
			if d.UDID == udid {
				return d.Name, nil
			}
		}
	}
	return "", nil
}

// iosStatusBarRegion returns the top strip of the captured PNG where the
// status bar lives. We use max(60, height/20) so small test images get
// enough height to fit a nonce at scale 3 (= 21px template height) and
// full-size simulator captures keep the scan region tight against the
// actual status bar.
func iosStatusBarRegion(pngBytes []byte) (image.Rectangle, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("decode png config: %w", err)
	}
	width := cfg.Width
	height := cfg.Height
	if width == 0 || height == 0 {
		return image.Rectangle{}, fmt.Errorf("screenshot has zero dimension: %dx%d", width, height)
	}
	strip := height / 20
	if strip < iosStatusBarMinHeight {
		strip = iosStatusBarMinHeight
	}
	if strip > height {
		strip = height
	}
	return image.Rect(0, 0, width, strip), nil
}

// verifyIOSNonce scans the provided region for the rendered nonce, trying
// scale=2 first and falling back to scale=3. Returns the scale that
// matched, or an error with the best attempt's details on failure.
func verifyIOSNonce(pngBytes []byte, nonce string, region image.Rectangle) (int, error) {
	text := "proctor:" + nonce
	var firstErr error
	for _, scale := range []int{2, 3} {
		err := VerifyNonceInRegion(pngBytes, text, region, iosNonceTolerance, scale)
		if err == nil {
			return scale, nil
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("scale=%d: %w", scale, err)
		} else {
			firstErr = fmt.Errorf("%s; scale=%d: %v", firstErr.Error(), scale, err)
		}
	}
	return 0, firstErr
}

// formatIOSRegion renders a rectangle as "x,y,width,height" so we can
// stash it in the capture ledger's Target map.
func formatIOSRegion(r image.Rectangle) string {
	return fmt.Sprintf("%d,%d,%d,%d", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

// runSimctlCmd is the real-world xcrun simctl invocation. It returns
// combined stdout/stderr on failure so callers can surface the underlying
// error text in their wrapper messages.
func runSimctlCmd(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"simctl"}, args...)
	cmd := exec.CommandContext(ctx, "xcrun", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		trimmed := strings.TrimSpace(stderr.String())
		if trimmed != "" {
			return nil, fmt.Errorf("%w: %s", err, trimmed)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

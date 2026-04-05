package proctor

import (
	"context"
	"fmt"
)

// captureStubBackend is a placeholder backend used for surfaces that do not
// yet have a real implementation. Real surface-specific backends register
// themselves in their own init() functions. The stub init() runs after
// them (capture_stubs.go sorts after every other capture_*.go file) and
// only fills in surfaces that remain unregistered, so real backends are
// never overwritten by stubs.
type captureStubBackend struct {
	surface string
}

func (s captureStubBackend) Capture(ctx context.Context, dest CaptureDestination, opts CaptureOptions) (CaptureResult, error) {
	return CaptureResult{}, fmt.Errorf("capture backend for %s not yet implemented", s.surface)
}

func init() {
	for _, surface := range []string{SurfaceBrowser, SurfaceIOS, SurfaceDesktop, SurfaceCLI} {
		if _, err := lookupCaptureBackend(surface); err == nil {
			continue
		}
		RegisterCaptureBackend(surface, captureStubBackend{surface: surface})
	}
}

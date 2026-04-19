// internal/proctor/profile_detect_test.go
package proctor

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectProfileWebFromPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"x","scripts":{"dev":"next dev"}}`)
	p, _ := DetectProfile(dir)
	if p.Platform != PlatformWeb {
		t.Fatalf("expected web, got %q", p.Platform)
	}
}

func TestDetectProfileIOSFromPodfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Podfile", `platform :ios, '15.0'`)
	p, _ := DetectProfile(dir)
	if p.Platform != PlatformIOS {
		t.Fatalf("expected ios, got %q", p.Platform)
	}
}

func TestDetectProfileIOSFromXcodeProj(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "App.xcodeproj"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, _ := DetectProfile(dir)
	if p.Platform != PlatformIOS {
		t.Fatalf("expected ios, got %q", p.Platform)
	}
}

func TestDetectProfileCLIFromGoMod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.25\n")
	p, _ := DetectProfile(dir)
	if p.Platform != PlatformCLI {
		t.Fatalf("expected cli, got %q", p.Platform)
	}
}

func TestDetectProfileUnknown(t *testing.T) {
	dir := t.TempDir()
	p, _ := DetectProfile(dir)
	if p.Platform != "" {
		t.Fatalf("expected empty platform, got %q", p.Platform)
	}
}

func TestDetectProfileURLFromNextDevPort(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"dev":"next dev -p 4000"}}`)
	p, _ := DetectProfile(dir)
	if p.Web == nil || p.Web.DevURL != "http://127.0.0.1:4000" {
		t.Fatalf("expected :4000, got %+v", p.Web)
	}
}

func TestDetectProfileURLFromViteDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"dev":"vite"}}`)
	p, _ := DetectProfile(dir)
	if p.Web == nil || p.Web.DevURL != "http://127.0.0.1:5173" {
		t.Fatalf("expected :5173 default, got %+v", p.Web)
	}
}

func TestDetectProfileURLFromNextDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"dev":"next dev"}}`)
	p, _ := DetectProfile(dir)
	if p.Web == nil || p.Web.DevURL != "http://127.0.0.1:3000" {
		t.Fatalf("expected :3000 default, got %+v", p.Web)
	}
}

func TestDetectProfileURLUnknownScriptLeavesEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"dev":"some-unknown-runner"}}`)
	p, _ := DetectProfile(dir)
	if p.Web == nil || p.Web.DevURL != "" {
		t.Fatalf("expected empty URL for unknown runner, got %+v", p.Web)
	}
}

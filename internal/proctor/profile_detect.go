// internal/proctor/profile_detect.go
package proctor

import (
	"os"
	"path/filepath"
)

// DetectProfile probes the repo root for signals and returns a best-guess
// profile plus a list of field dotted-names that were auto-populated (used by
// the CLI to explain what was detected vs empty).
func DetectProfile(repoRoot string) (Profile, []string) {
	p := Profile{Version: ProfileVersion}
	var detected []string
	if fileExists(filepath.Join(repoRoot, "package.json")) {
		p.Platform = PlatformWeb
		p.Web = &WebProfile{}
		detected = append(detected, "platform")
		return p, detected
	}
	if fileExists(filepath.Join(repoRoot, "Podfile")) || hasXcodeProj(repoRoot) {
		p.Platform = PlatformIOS
		p.IOS = &IOSProfile{}
		detected = append(detected, "platform")
		return p, detected
	}
	if fileExists(filepath.Join(repoRoot, "go.mod")) {
		p.Platform = PlatformCLI
		p.CLI = &CLIProfile{}
		detected = append(detected, "platform")
		return p, detected
	}
	return p, detected
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func hasXcodeProj(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && filepath.Ext(entry.Name()) == ".xcodeproj" {
			return true
		}
	}
	return false
}

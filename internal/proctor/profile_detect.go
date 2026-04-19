// internal/proctor/profile_detect.go
package proctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectProfile probes the repo root for signals and returns a best-guess
// profile plus a list of field dotted-names that were auto-populated (used by
// the CLI to explain what was detected vs empty).
func DetectProfile(repoRoot string) (Profile, []string) {
	p := Profile{Version: ProfileVersion}
	var detected []string
	if fileExists(filepath.Join(repoRoot, "package.json")) {
		p.Platform = PlatformWeb
		web := &WebProfile{}
		if url := detectWebDevURL(filepath.Join(repoRoot, "package.json")); url != "" {
			web.DevURL = url
			detected = append(detected, "web.dev_url")
		}
		p.Web = web
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

var portFlagRegexp = regexp.MustCompile(`\s-p(?:\s+|=)(\d{2,5})|--port(?:\s+|=)(\d{2,5})`)

func detectWebDevURL(packageJSONPath string) string {
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return ""
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	dev := pkg.Scripts["dev"]
	if dev == "" {
		return ""
	}
	if matches := portFlagRegexp.FindStringSubmatch(dev); matches != nil {
		for _, m := range matches[1:] {
			if m != "" {
				return "http://127.0.0.1:" + m
			}
		}
	}
	switch {
	case strings.Contains(dev, "next"):
		return "http://127.0.0.1:3000"
	case strings.Contains(dev, "vite"):
		return "http://127.0.0.1:5173"
	}
	return ""
}

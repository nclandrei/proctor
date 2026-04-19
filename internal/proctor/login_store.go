// internal/proctor/login_store.go
package proctor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const DefaultLoginTTL = "12h"

// SaveLogin copies the source file into ~/.proctor/profiles/<slug>/session.json,
// records mtime/hash on the profile, and persists the profile. Web platform only.
// ttlOverride of "" keeps the existing ttl (or DefaultLoginTTL if unset).
func SaveLogin(s *Store, slug, srcPath, ttlOverride string) (Profile, error) {
	p, err := LoadProfile(s, slug)
	if err != nil {
		return Profile{}, err
	}
	if p.Platform != PlatformWeb {
		return Profile{}, fmt.Errorf("login save requires platform=web (current: %q)", p.Platform)
	}
	if p.Web == nil {
		p.Web = &WebProfile{}
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return Profile{}, err
	}
	defer src.Close()

	destPath := filepath.Join(s.ProfileDir(slug), "session.json")
	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Profile{}, err
	}
	hasher := sha256.New()
	writer := io.MultiWriter(dst, hasher)
	if _, err := io.Copy(writer, src); err != nil {
		dst.Close()
		os.Remove(destPath)
		return Profile{}, err
	}
	if err := dst.Close(); err != nil {
		return Profile{}, err
	}

	ttl := DefaultLoginTTL
	if p.Web.Login != nil && p.Web.Login.TTL != "" {
		ttl = p.Web.Login.TTL
	}
	if ttlOverride != "" {
		ttl = ttlOverride
	}
	p.Web.Login = &LoginConfig{
		File:    "session.json",
		TTL:     ttl,
		SavedAt: time.Now().UTC().Format(time.RFC3339),
		SHA256:  hex.EncodeToString(hasher.Sum(nil)),
	}
	if err := SaveProfile(s, slug, p); err != nil {
		return Profile{}, err
	}
	return p, nil
}

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
	// Pre-check platform gate using the current profile so we don't copy the
	// session file for non-web platforms.
	existing, err := LoadProfile(s, slug)
	if err != nil {
		return Profile{}, err
	}
	if existing.Platform != PlatformWeb {
		return Profile{}, fmt.Errorf("login save requires platform=web (current: %q)", existing.Platform)
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
	sum := hex.EncodeToString(hasher.Sum(nil))

	updated, err := UpdateProfile(s, slug, func(p *Profile) error {
		if p.Platform != PlatformWeb {
			return fmt.Errorf("login save requires platform=web (current: %q)", p.Platform)
		}
		if p.Web == nil {
			p.Web = &WebProfile{}
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
			SHA256:  sum,
		}
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	return updated, nil
}

type LoginKind string

const (
	LoginMissing LoginKind = "missing"
	LoginFresh   LoginKind = "fresh"
	LoginStale   LoginKind = "stale"
	LoginCorrupt LoginKind = "corrupt"
)

type LoginState struct {
	Kind    LoginKind
	Age     time.Duration
	TTL     time.Duration
	SavedAt time.Time
}

func LoginStateFor(s *Store, slug string) (LoginState, error) {
	p, err := LoadProfile(s, slug)
	if err != nil {
		return LoginState{}, err
	}
	return LoginStateForProfile(s, p), nil
}

func LoginStateForProfile(s *Store, p Profile) LoginState {
	if p.Web == nil || p.Web.Login == nil || p.Web.Login.SavedAt == "" || p.Web.Login.SHA256 == "" {
		return LoginState{Kind: LoginMissing}
	}
	savedAt, err := time.Parse(time.RFC3339, p.Web.Login.SavedAt)
	if err != nil {
		return LoginState{Kind: LoginMissing}
	}
	destPath := filepath.Join(s.ProfileDir(p.RepoSlug), "session.json")
	tightenPermsIfLoose(destPath)
	data, err := os.ReadFile(destPath)
	if err != nil {
		return LoginState{Kind: LoginMissing}
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != p.Web.Login.SHA256 {
		return LoginState{Kind: LoginCorrupt, SavedAt: savedAt}
	}
	ttl, err := time.ParseDuration(p.Web.Login.TTL)
	if err != nil || ttl <= 0 {
		ttl, _ = time.ParseDuration(DefaultLoginTTL)
	}
	age := time.Since(savedAt)
	state := LoginState{Age: age, TTL: ttl, SavedAt: savedAt}
	if age > ttl {
		state.Kind = LoginStale
	} else {
		state.Kind = LoginFresh
	}
	return state
}

func InvalidateLogin(s *Store, slug string) (Profile, error) {
	existing, err := LoadProfile(s, slug)
	if err != nil {
		return Profile{}, err
	}
	if existing.Platform != PlatformWeb {
		return Profile{}, fmt.Errorf("login invalidate requires platform=web (current: %q)", existing.Platform)
	}
	destPath := filepath.Join(s.ProfileDir(slug), "session.json")
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return Profile{}, err
	}
	updated, err := UpdateProfile(s, slug, func(p *Profile) error {
		if p.Platform != PlatformWeb {
			return fmt.Errorf("login invalidate requires platform=web (current: %q)", p.Platform)
		}
		if p.Web != nil && p.Web.Login != nil {
			p.Web.Login.SavedAt = ""
			p.Web.Login.SHA256 = ""
		}
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	return updated, nil
}

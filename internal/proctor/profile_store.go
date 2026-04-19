// internal/proctor/profile_store.go
package proctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func (s *Store) ProfilesRoot() string {
	return filepath.Join(s.Home, "profiles")
}

func (s *Store) ProfileDir(slug string) string {
	return filepath.Join(s.ProfilesRoot(), slug)
}

func (s *Store) profilePath(slug string) string {
	return filepath.Join(s.ProfileDir(slug), "profile.json")
}

func LoadProfile(s *Store, slug string) (Profile, error) {
	data, err := os.ReadFile(s.profilePath(slug))
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("parse profile %s: %w", s.profilePath(slug), err)
	}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

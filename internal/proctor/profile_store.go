// internal/proctor/profile_store.go
package proctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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

// SaveProfile writes the profile atomically with 0600 perms. Recomputes
// MissingFieldsList/Incomplete before writing. An exclusive flock is held on
// the destination during the read-merge-write cycle so concurrent init/set
// calls do not clobber each other.
func SaveProfile(s *Store, slug string, p Profile) error {
	if p.Version == 0 {
		p.Version = ProfileVersion
	}
	p.RepoSlug = slug
	p.Recompute()
	dir := s.ProfileDir(slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	dest := s.profilePath(slug)
	lock, err := os.OpenFile(dest+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock profile: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	data, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, "profile-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

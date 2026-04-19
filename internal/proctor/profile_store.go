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
	path := s.profilePath(slug)
	tightenPermsIfLoose(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("parse profile %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// tightenPermsIfLoose checks the file at path and, if its mode contains any
// group or world permission bits, prints a warning to stderr and attempts a
// best-effort chmod to 0600. Missing files and stat failures are ignored.
func tightenPermsIfLoose(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	mode := info.Mode().Perm()
	if mode&0o077 == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: %s had permissions %04o; tightening to 0600\n", path, mode)
	_ = os.Chmod(path, 0o600)
}

// saveProfileLocked writes the profile atomically with 0600 perms. It assumes
// the caller already holds the flock on profile.json.lock and that the profile
// directory already exists.
func saveProfileLocked(s *Store, slug string, p Profile) error {
	if p.Version == 0 {
		p.Version = ProfileVersion
	}
	p.RepoSlug = slug
	p.Recompute()
	dir := s.ProfileDir(slug)
	dest := s.profilePath(slug)
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

// SaveProfile writes the profile atomically with 0600 perms. Recomputes
// MissingFieldsList/Incomplete before writing. An exclusive flock is held on
// the destination during the write so concurrent init/set calls do not clobber
// each other.
func SaveProfile(s *Store, slug string, p Profile) error {
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
	return saveProfileLocked(s, slug, p)
}

// UpdateProfile serializes a read-mutate-write cycle under the profile flock
// so concurrent callers cannot lose each other's updates. If the profile does
// not yet exist a zero Profile is passed to mutate. The returned Profile is
// the persisted value.
func UpdateProfile(s *Store, slug string, mutate func(*Profile) error) (Profile, error) {
	dir := s.ProfileDir(slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Profile{}, err
	}
	dest := s.profilePath(slug)
	lock, err := os.OpenFile(dest+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Profile{}, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Profile{}, fmt.Errorf("lock profile: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	var p Profile
	loaded, err := LoadProfile(s, slug)
	if err != nil && !os.IsNotExist(err) {
		return Profile{}, err
	}
	if err == nil {
		p = loaded
	}
	if mutate != nil {
		if err := mutate(&p); err != nil {
			return Profile{}, err
		}
	}
	if err := saveProfileLocked(s, slug, p); err != nil {
		return Profile{}, err
	}
	// Return what's actually on disk (including Recompute results).
	reloaded, err := LoadProfile(s, slug)
	if err != nil {
		return Profile{}, err
	}
	return reloaded, nil
}

// internal/proctor/profile_store_test.go
package proctor

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Home: t.TempDir()}
}

func TestProfilesRootAndDir(t *testing.T) {
	s := newTestStore(t)
	if got, want := s.ProfilesRoot(), filepath.Join(s.Home, "profiles"); got != want {
		t.Fatalf("ProfilesRoot: got %q want %q", got, want)
	}
	if got, want := s.ProfileDir("nclandrei-proctor"), filepath.Join(s.Home, "profiles", "nclandrei-proctor"); got != want {
		t.Fatalf("ProfileDir: got %q want %q", got, want)
	}
}

func TestLoadProfileMissing(t *testing.T) {
	s := newTestStore(t)
	_, err := LoadProfile(s, "nclandrei-proctor")
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestLoadProfileExisting(t *testing.T) {
	s := newTestStore(t)
	dir := s.ProfileDir("nclandrei-proctor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"version":1,"platform":"web","web":{"dev_url":"http://x","test_email":"a@b.c","test_password":"p"}}`)
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(s, "nclandrei-proctor")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.Version != 1 || p.Platform != "web" || p.Web.DevURL != "http://x" {
		t.Fatalf("loaded profile: %+v", p)
	}
}

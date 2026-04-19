// internal/proctor/profile_store_test.go
package proctor

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestSaveProfileCreatesFileAndDir(t *testing.T) {
	s := newTestStore(t)
	p := Profile{Version: 1, RepoSlug: "r", Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://x", TestEmail: "a@b.c", TestPassword: "p",
	}}
	if err := SaveProfile(s, "r", p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	info, err := os.Stat(s.profilePath("r"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode: got %o want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(s.ProfileDir("r"))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if runtime.GOOS != "windows" && dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode: got %o want 0700", dirInfo.Mode().Perm())
	}
	loaded, err := LoadProfile(s, "r")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if loaded.Web.TestEmail != "a@b.c" {
		t.Fatalf("round-trip lost data: %+v", loaded)
	}
}

func TestSaveProfileRecomputesIncomplete(t *testing.T) {
	s := newTestStore(t)
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{DevURL: "http://x"}}
	if err := SaveProfile(s, "r", p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	loaded, _ := LoadProfile(s, "r")
	if !loaded.Incomplete {
		t.Fatalf("expected Incomplete=true after save, got %+v", loaded)
	}
	if len(loaded.MissingFieldsList) < 2 {
		t.Fatalf("expected missing fields, got %v", loaded.MissingFieldsList)
	}
}

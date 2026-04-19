// internal/proctor/login_store_test.go
package proctor

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoginCopiesFileAndUpdatesProfile(t *testing.T) {
	s := newTestStore(t)
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://x", TestEmail: "a@b.c", TestPassword: "p",
	}}
	if err := SaveProfile(s, "r", p); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "storage.json")
	content := []byte(`{"cookies":[]}`)
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := SaveLogin(s, "r", src, "")
	if err != nil {
		t.Fatalf("SaveLogin: %v", err)
	}
	if updated.Web.Login == nil || updated.Web.Login.File != "session.json" {
		t.Fatalf("login config not set: %+v", updated.Web.Login)
	}
	sum := sha256.Sum256(content)
	if updated.Web.Login.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash mismatch: got %q", updated.Web.Login.SHA256)
	}
	if updated.Web.Login.SavedAt == "" {
		t.Fatalf("SavedAt should be set")
	}
	copied, err := os.ReadFile(filepath.Join(s.ProfileDir("r"), "session.json"))
	if err != nil {
		t.Fatalf("read copied session: %v", err)
	}
	if string(copied) != string(content) {
		t.Fatalf("copy mismatch")
	}
	persisted, _ := LoadProfile(s, "r")
	if persisted.Web.Login == nil || persisted.Web.Login.SHA256 == "" {
		t.Fatalf("profile not persisted: %+v", persisted.Web.Login)
	}
}

func TestSaveLoginRejectsNonWeb(t *testing.T) {
	s := newTestStore(t)
	p := Profile{Version: 1, Platform: PlatformIOS, IOS: &IOSProfile{Scheme: "X", BundleID: "x", Simulator: "x"}}
	if err := SaveProfile(s, "r", p); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(src, []byte("{}"), 0o644)
	if _, err := SaveLogin(s, "r", src, ""); err == nil {
		t.Fatalf("expected error for non-web platform")
	}
}

func TestSaveLoginOverridesTTL(t *testing.T) {
	s := newTestStore(t)
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://x", TestEmail: "a@b.c", TestPassword: "p",
	}}
	if err := SaveProfile(s, "r", p); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(src, []byte("{}"), 0o644)
	updated, err := SaveLogin(s, "r", src, "2h")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Web.Login.TTL != "2h" {
		t.Fatalf("ttl override: got %q want 2h", updated.Web.Login.TTL)
	}
}

func TestInvalidateLoginRemovesFileAndConfig(t *testing.T) {
	s := newTestStore(t)
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://x", TestEmail: "a@b.c", TestPassword: "p",
	}}
	SaveProfile(s, "r", p)
	src := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(src, []byte("{}"), 0o644)
	SaveLogin(s, "r", src, "")

	updated, err := InvalidateLogin(s, "r")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.ProfileDir("r"), "session.json")); !os.IsNotExist(err) {
		t.Fatalf("session.json should be gone, err=%v", err)
	}
	if updated.Web.Login != nil && (updated.Web.Login.SavedAt != "" || updated.Web.Login.SHA256 != "") {
		t.Fatalf("login fields should be cleared: %+v", updated.Web.Login)
	}
}

func TestInvalidateLoginNoopWhenMissing(t *testing.T) {
	s := newTestStore(t)
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://x", TestEmail: "a@b.c", TestPassword: "p",
	}}
	SaveProfile(s, "r", p)
	if _, err := InvalidateLogin(s, "r"); err != nil {
		t.Fatalf("expected no error when login missing, got %v", err)
	}
}

// internal/proctor/profile_test.go
package proctor

import (
	"encoding/json"
	"testing"
)

func TestProfileRoundTrip(t *testing.T) {
	original := Profile{
		Version:  1,
		RepoSlug: "nclandrei-proctor",
		Platform: PlatformWeb,
		Web: &WebProfile{
			DevURL:       "http://127.0.0.1:3000",
			AuthURL:      "POST /api/session",
			TestEmail:    "demo@example.com",
			TestPassword: "hunter2",
			Login: &LoginConfig{
				File: "session.json",
				TTL:  "12h",
			},
		},
	}
	data, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round Profile
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Version != 1 || round.Platform != PlatformWeb {
		t.Fatalf("round-trip lost header fields: %+v", round)
	}
	if round.Web == nil || round.Web.TestEmail != "demo@example.com" || round.Web.Login == nil || round.Web.Login.TTL != "12h" {
		t.Fatalf("round-trip lost web fields: %+v", round.Web)
	}
}

func TestProfileRejectsUnknownVersion(t *testing.T) {
	raw := []byte(`{"version":2,"platform":"web"}`)
	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := p.Validate(); err == nil {
		t.Fatalf("expected error for version 2, got nil")
	}
}

func TestProfileMissingFieldsWebComplete(t *testing.T) {
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://127.0.0.1:3000", TestEmail: "demo@example.com", TestPassword: "x",
	}}
	if got := p.MissingFields(); len(got) != 0 {
		t.Fatalf("expected no missing fields, got %v", got)
	}
	if p.IsIncomplete() {
		t.Fatalf("expected complete, got incomplete")
	}
}

func TestProfileMissingFieldsWebPartial(t *testing.T) {
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{DevURL: "http://x"}}
	got := p.MissingFields()
	want := []string{"web.test_email", "web.test_password"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("missing fields: got %v want %v", got, want)
	}
	if !p.IsIncomplete() {
		t.Fatalf("expected incomplete")
	}
}

func TestProfileMissingFieldsIOS(t *testing.T) {
	p := Profile{Version: 1, Platform: PlatformIOS, IOS: &IOSProfile{Scheme: "X"}}
	got := p.MissingFields()
	want := []string{"ios.bundle_id", "ios.simulator"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("missing fields: got %v want %v", got, want)
	}
}

func TestProfileMissingFieldsNoPlatform(t *testing.T) {
	p := Profile{Version: 1}
	got := p.MissingFields()
	if len(got) != 1 || got[0] != "platform" {
		t.Fatalf("missing fields: got %v", got)
	}
}

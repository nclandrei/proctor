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

func TestProfileRedacted(t *testing.T) {
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://x", TestEmail: "demo@example.com", TestPassword: "hunter2",
	}}
	r := p.Redacted()
	if r.Web.TestPassword != "***" {
		t.Fatalf("expected password redacted, got %q", r.Web.TestPassword)
	}
	if r.Web.TestEmail != "demo@example.com" {
		t.Fatalf("email should not be redacted, got %q", r.Web.TestEmail)
	}
	if p.Web.TestPassword != "hunter2" {
		t.Fatalf("redaction should not mutate receiver")
	}
}

func TestProfileRedactedNoSecret(t *testing.T) {
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{DevURL: "http://x"}}
	r := p.Redacted()
	if r.Web.TestPassword != "" {
		t.Fatalf("empty password should redact to empty, got %q", r.Web.TestPassword)
	}
}

func TestProfileFieldValueWeb(t *testing.T) {
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{
		DevURL: "http://x", TestPassword: "hunter2",
	}}
	cases := map[string]string{
		"platform":          "web",
		"web.dev_url":       "http://x",
		"web.test_password": "hunter2",
	}
	for field, want := range cases {
		got, err := p.FieldValue(field)
		if err != nil {
			t.Fatalf("%s: %v", field, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", field, got, want)
		}
	}
}

func TestProfileFieldValueUnknown(t *testing.T) {
	p := Profile{Version: 1, Platform: PlatformWeb, Web: &WebProfile{}}
	if _, err := p.FieldValue("web.nope"); err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

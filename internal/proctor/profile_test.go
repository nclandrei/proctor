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

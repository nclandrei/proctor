// internal/proctor/profile.go
package proctor

import "fmt"

const ProfileVersion = 1

type Profile struct {
	Version           int             `json:"version"`
	RepoSlug          string          `json:"repo_slug,omitempty"`
	Platform          string          `json:"platform,omitempty"`
	Incomplete        bool            `json:"incomplete"`
	MissingFieldsList []string        `json:"missing_fields,omitempty"`
	Web               *WebProfile     `json:"web,omitempty"`
	IOS               *IOSProfile     `json:"ios,omitempty"`
	Desktop           *DesktopProfile `json:"desktop,omitempty"`
	CLI               *CLIProfile     `json:"cli,omitempty"`
}

type WebProfile struct {
	DevURL       string       `json:"dev_url,omitempty"`
	AuthURL      string       `json:"auth_url,omitempty"`
	TestEmail    string       `json:"test_email,omitempty"`
	TestPassword string       `json:"test_password,omitempty"`
	Login        *LoginConfig `json:"login,omitempty"`
}

type LoginConfig struct {
	File    string `json:"file,omitempty"`
	TTL     string `json:"ttl,omitempty"`
	SavedAt string `json:"saved_at,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

type IOSProfile struct {
	Scheme    string `json:"scheme,omitempty"`
	BundleID  string `json:"bundle_id,omitempty"`
	Simulator string `json:"simulator,omitempty"`
}

type DesktopProfile struct {
	AppName  string `json:"app_name,omitempty"`
	BundleID string `json:"bundle_id,omitempty"`
}

type CLIProfile struct {
	Command string `json:"command,omitempty"`
}

func (p *Profile) Validate() error {
	if p.Version != ProfileVersion {
		return fmt.Errorf("profile version %d unsupported by this proctor (expected %d)", p.Version, ProfileVersion)
	}
	return nil
}

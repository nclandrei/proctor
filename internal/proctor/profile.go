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

func (p *Profile) MissingFields() []string {
	if p.Platform == "" {
		return []string{"platform"}
	}
	var missing []string
	switch p.Platform {
	case PlatformWeb:
		w := p.Web
		if w == nil {
			w = &WebProfile{}
		}
		if w.DevURL == "" {
			missing = append(missing, "web.dev_url")
		}
		if w.TestEmail == "" {
			missing = append(missing, "web.test_email")
		}
		if w.TestPassword == "" {
			missing = append(missing, "web.test_password")
		}
	case PlatformIOS:
		i := p.IOS
		if i == nil {
			i = &IOSProfile{}
		}
		if i.Scheme == "" {
			missing = append(missing, "ios.scheme")
		}
		if i.BundleID == "" {
			missing = append(missing, "ios.bundle_id")
		}
		if i.Simulator == "" {
			missing = append(missing, "ios.simulator")
		}
	case PlatformDesktop:
		d := p.Desktop
		if d == nil {
			d = &DesktopProfile{}
		}
		if d.AppName == "" {
			missing = append(missing, "desktop.app_name")
		}
		if d.BundleID == "" {
			missing = append(missing, "desktop.bundle_id")
		}
	case PlatformCLI:
		c := p.CLI
		if c == nil {
			c = &CLIProfile{}
		}
		if c.Command == "" {
			missing = append(missing, "cli.command")
		}
	default:
		return []string{"platform"}
	}
	return missing
}

func (p *Profile) IsIncomplete() bool {
	return len(p.MissingFields()) > 0
}

// Recompute mirrors MissingFields() into the serialized fields so a saved
// profile tells the truth about its own completeness.
func (p *Profile) Recompute() {
	missing := p.MissingFields()
	p.MissingFieldsList = missing
	p.Incomplete = len(missing) > 0
}

// Redacted returns a deep copy of the profile with secret fields replaced by
// "***" where non-empty. Used only for human-facing display (project show).
// proctor project get reads the raw value directly; it never goes through this.
func (p *Profile) Redacted() Profile {
	copied := *p
	if p.Web != nil {
		w := *p.Web
		if w.TestPassword != "" {
			w.TestPassword = "***"
		}
		if w.Login != nil {
			loginCopy := *w.Login
			w.Login = &loginCopy
		}
		copied.Web = &w
	}
	if p.IOS != nil {
		iosCopy := *p.IOS
		copied.IOS = &iosCopy
	}
	if p.Desktop != nil {
		deskCopy := *p.Desktop
		copied.Desktop = &deskCopy
	}
	if p.CLI != nil {
		cliCopy := *p.CLI
		copied.CLI = &cliCopy
	}
	if len(p.MissingFieldsList) > 0 {
		copied.MissingFieldsList = append([]string(nil), p.MissingFieldsList...)
	}
	return copied
}

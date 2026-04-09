package proctor

import (
	"strings"
	"time"
)

const (
	PlatformWeb     = "web"
	PlatformIOS     = "ios"
	PlatformCLI     = "cli"
	PlatformDesktop = "desktop"

	SurfaceBrowser = "browser"
	SurfaceIOS     = "ios"
	SurfaceCurl    = "curl"
	SurfaceCLI     = "cli"
	SurfaceDesktop = "desktop"

	CurlModeRequired = "required"
	CurlModeScenario = "scenario"
	CurlModeSkip     = "skip"

	TierWrappedCommand = 2
	TierRegisteredRun  = 3
	StatusInProgress   = "in_progress"
	StatusBlocked      = "blocked"
	StatusPassed       = "passed"
	AssertionPass      = "pass"
	AssertionFail      = "fail"

	EvidenceStatusPending  = "pending-verification"
	EvidenceStatusComplete = "complete"

	MinVerdictLength   = 40
	MinPreNoteLength   = 20
	MinActionLength    = 20
	MinDistinctWords   = 4
	ArtifactImage      = "image"
	ArtifactJSONReport = "json-report"
	ArtifactTranscript = "transcript"
	ArtifactHTML       = "html"
	ArtifactMarkdown   = "markdown"
	EdgeCategoryNA     = "na"
	EdgeCategoryScenar = "scenario"
)

var WebEdgeCaseCategories = []string{
	"validation and malformed input",
	"empty or missing input",
	"retry or double-submit",
	"loading, latency, and race conditions",
	"network or server failure",
	"auth and session state",
	"refresh, back-navigation, and state persistence",
	"mobile or responsive behavior",
	"accessibility and keyboard behavior",
	"any feature-specific risks",
}

var IOSEdgeCaseCategories = []string{
	"validation and malformed input",
	"empty or missing input",
	"retry or double-submit",
	"loading, latency, and race conditions",
	"network or server failure",
	"auth and session state",
	"app lifecycle, relaunch, and state persistence",
	"device traits, orientation, and layout",
	"accessibility, dynamic type, and keyboard behavior",
	"any feature-specific risks",
}

var CLIEdgeCaseCategories = []string{
	"invalid or malformed input",
	"missing required args, files, config, or env",
	"retry, rerun, and idempotency",
	"long-running output, streaming, or progress state",
	"interrupts, cancellation, and signals",
	"tty, pipe, and non-interactive behavior",
	"terminal layout, wrapping, and resize behavior",
	"keyboard navigation and shortcut behavior",
	"state, config, and persistence across reruns",
	"stderr, exit codes, and partial failure reporting",
	"any feature-specific risks",
}

var DesktopEdgeCaseCategories = []string{
	"validation and malformed input",
	"empty or missing input",
	"retry or double-submit",
	"loading, latency, and race conditions",
	"network or server failure",
	"auth and session state",
	"window management, resize, and multi-monitor",
	"drag-drop, clipboard, and system integration",
	"keyboard shortcuts and accessibility",
	"any feature-specific risks",
}

var EdgeCaseCategories = append([]string(nil), WebEdgeCaseCategories...)

func EdgeCaseCategoriesForPlatform(platform string) []string {
	switch normalizePlatform(platform) {
	case PlatformIOS:
		return append([]string(nil), IOSEdgeCaseCategories...)
	case PlatformCLI:
		return append([]string(nil), CLIEdgeCaseCategories...)
	case PlatformDesktop:
		return append([]string(nil), DesktopEdgeCaseCategories...)
	default:
		return append([]string(nil), WebEdgeCaseCategories...)
	}
}

func NormalizePlatform(platform string) string {
	return normalizePlatform(platform)
}

type Run struct {
	ID                 string             `json:"id"`
	RepoSlug           string             `json:"repo_slug"`
	RepoRoot           string             `json:"repo_root"`
	Platform           string             `json:"platform"`
	Surface            string             `json:"surface"`
	Feature            string             `json:"feature"`
	BrowserURL         string             `json:"browser_url"`
	CLICommand         string             `json:"cli_command,omitempty"`
	Desktop            DesktopApp         `json:"desktop_app,omitempty"`
	CurlMode           string             `json:"curl_mode,omitempty"`
	IOS                IOSTarget          `json:"ios,omitempty"`
	CurlRequired       bool               `json:"curl_required"`
	CurlEndpoints      []string           `json:"curl_endpoints,omitempty"`
	CurlSkipReason     string             `json:"curl_skip_reason,omitempty"`
	HappyPath          Scenario           `json:"happy_path"`
	FailurePath        Scenario           `json:"failure_path"`
	EdgeCaseCategories []EdgeCaseCategory `json:"edge_case_categories"`
	Scenarios          []Scenario         `json:"scenarios"`
	Status             string             `json:"status"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

type Scenario struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Kind            string   `json:"kind"`
	Category        string   `json:"category,omitempty"`
	BrowserRequired bool     `json:"browser_required"`
	IOSRequired     bool     `json:"ios_required,omitempty"`
	CurlRequired    bool     `json:"curl_required"`
	CLIRequired     bool     `json:"cli_required"`
	DesktopRequired bool     `json:"desktop_required,omitempty"`
	CurlEndpoints   []string `json:"curl_endpoints,omitempty"`
}

type EdgeCaseCategory struct {
	Category    string   `json:"category"`
	Status      string   `json:"status"`
	Reason      string   `json:"reason,omitempty"`
	ScenarioIDs []string `json:"scenario_ids,omitempty"`
}

type Evidence struct {
	ID         string       `json:"id"`
	RunID      string       `json:"run_id"`
	ScenarioID string       `json:"scenario_id"`
	Surface    string       `json:"surface"`
	Tier       int          `json:"tier"`
	CreatedAt  time.Time    `json:"created_at"`
	Title      string       `json:"title"`
	Provenance Provenance   `json:"provenance"`
	Assertions []Assertion  `json:"assertions"`
	Artifacts  []Artifact   `json:"artifacts"`
	CaptureIDs []string     `json:"capture_ids,omitempty"`
	Browser    *BrowserData `json:"browser,omitempty"`
	IOS        *IOSData     `json:"ios,omitempty"`
	Curl       *CurlData    `json:"curl,omitempty"`
	CLI        *CLIData     `json:"cli,omitempty"`
	Desktop    *DesktopData `json:"desktop,omitempty"`
	Status     string       `json:"status,omitempty"`
	// Notes stores the verification verdict. The JSON key stays "notes" for
	// backwards compatibility with existing evidence.jsonl files.
	Notes      string     `json:"notes,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}

type Provenance struct {
	Mode       string   `json:"mode"`
	Tool       string   `json:"tool"`
	SessionID  string   `json:"session_id,omitempty"`
	Command    []string `json:"command,omitempty"`
	CWD        string   `json:"cwd"`
	RecordedBy string   `json:"recorded_by"`
}

type PreNote struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	Scenario  string    `json:"scenario"`
	Session   string    `json:"session"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

type Assertion struct {
	Description string `json:"description"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
	Result      string `json:"result"`
	Message     string `json:"message,omitempty"`
}

type Artifact struct {
	Kind        string    `json:"kind"`
	Label       string    `json:"label"`
	Path        string    `json:"path"`
	SHA256      string    `json:"sha256"`
	Source      string    `json:"source,omitempty"`
	MediaType   string    `json:"media_type,omitempty"`
	SourceMtime time.Time `json:"source_mtime,omitempty"`
}

type IOSTarget struct {
	Scheme    string `json:"scheme,omitempty"`
	BundleID  string `json:"bundle_id,omitempty"`
	Simulator string `json:"simulator,omitempty"`
}

type BrowserData struct {
	URL       string                `json:"url"`
	SessionID string                `json:"session_id"`
	Tool      string                `json:"tool"`
	Desktop   BrowserDeviceSummary  `json:"desktop"`
	Mobile    *BrowserDeviceSummary `json:"mobile,omitempty"`
}

type BrowserDeviceSummary struct {
	Title           string `json:"title"`
	FinalURL        string `json:"final_url"`
	ConsoleErrors   int    `json:"console_errors"`
	ConsoleWarnings int    `json:"console_warnings"`
	PageErrors      int    `json:"page_errors"`
	FailedRequests  int    `json:"failed_requests"`
	HTTPErrors      int    `json:"http_errors"`
}

type IOSData struct {
	BundleID   string              `json:"bundle_id"`
	Screen     string              `json:"screen"`
	State      string              `json:"state,omitempty"`
	AppLaunch  bool                `json:"app_launch"`
	LaunchArgs []string            `json:"launch_args,omitempty"`
	Simulator  IOSSimulatorSummary `json:"simulator"`
	Issues     IOSIssueSummary     `json:"issues"`
}

type IOSSimulatorSummary struct {
	Name    string `json:"name"`
	UDID    string `json:"udid,omitempty"`
	Runtime string `json:"runtime,omitempty"`
}

type IOSIssueSummary struct {
	LaunchErrors int `json:"launch_errors"`
	Crashes      int `json:"crashes"`
	FatalLogs    int `json:"fatal_logs"`
}

type CurlData struct {
	Command        []string          `json:"command"`
	ExitCode       int               `json:"exit_code"`
	ResponseStatus int               `json:"response_status,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           string            `json:"body,omitempty"`
}

type CLIData struct {
	Command           string `json:"command"`
	SessionID         string `json:"session_id"`
	Tool              string `json:"tool"`
	ExitCode          *int   `json:"exit_code,omitempty"`
	TranscriptPreview string `json:"transcript_preview,omitempty"`
}

type DesktopApp struct {
	Name     string `json:"name,omitempty"`
	BundleID string `json:"bundle_id,omitempty"`
}

type DesktopData struct {
	AppName     string              `json:"app_name"`
	BundleID    string              `json:"bundle_id,omitempty"`
	PID         int                 `json:"pid,omitempty"`
	State       string              `json:"state,omitempty"`
	WindowTitle string              `json:"window_title,omitempty"`
	Tool        string              `json:"tool"`
	SessionID   string              `json:"session_id"`
	Issues      DesktopIssueSummary `json:"issues"`
}

type DesktopIssueSummary struct {
	Crashes   int `json:"crashes"`
	FatalLogs int `json:"fatal_logs"`
}

type StartOptions struct {
	Platform        string
	Surface         string
	Feature         string
	BrowserURL      string
	CLICommand      string
	IOSScheme       string
	IOSBundleID     string
	IOSSimulator    string
	DesktopAppName  string
	DesktopBundleID string
	CurlMode        string
	CurlEndpoints   []string
	CurlSkipReason  string
	HappyPath       string
	FailurePath     string
	EdgeCaseInputs  []string
}

type BrowserRecordOptions struct {
	ScenarioID       string
	SessionID        string
	Tool             string
	Screenshots      map[string]string
	ReportPath       string
	PassAssertions   []string
	FailAssertions   []string
	MaxScreenshotAge time.Duration
}

type IOSRecordOptions struct {
	ScenarioID       string
	SessionID        string
	Tool             string
	Screenshots      map[string]string
	ReportPath       string
	PassAssertions   []string
	FailAssertions   []string
	MaxScreenshotAge time.Duration
}

type DesktopRecordOptions struct {
	ScenarioID       string
	SessionID        string
	Tool             string
	Screenshots      map[string]string
	ReportPath       string
	PassAssertions   []string
	FailAssertions   []string
	MaxScreenshotAge time.Duration
}

type CurlRecordOptions struct {
	ScenarioID     string
	Command        []string
	PassAssertions []string
	FailAssertions []string
}

type CLIRecordOptions struct {
	ScenarioID       string
	SessionID        string
	Tool             string
	Command          string
	TranscriptPath   string
	ExitCode         *int
	Screenshots      map[string]string
	PassAssertions   []string
	FailAssertions   []string
	MaxScreenshotAge time.Duration
}

type Evaluation struct {
	Complete            bool
	ScenarioEvaluations []ScenarioEvaluation
	GlobalMissing       []string
}

type ScenarioEvaluation struct {
	Scenario      Scenario
	BrowserOK     bool
	IOSOK         bool
	CurlOK        bool
	CLIOK         bool
	DesktopOK     bool
	LogOK         bool
	BrowserIssues []string
	IOSIssues     []string
	CurlIssues    []string
	CLIIssues     []string
	DesktopIssues []string
	LogIssues     []string
}

func (s Scenario) RequiredSurfaces() []string {
	var surfaces []string
	if s.BrowserRequired {
		surfaces = append(surfaces, SurfaceBrowser)
	}
	if s.IOSRequired {
		surfaces = append(surfaces, SurfaceIOS)
	}
	if s.CurlRequired {
		surfaces = append(surfaces, SurfaceCurl)
	}
	if s.CLIRequired {
		surfaces = append(surfaces, SurfaceCLI)
	}
	if s.DesktopRequired {
		surfaces = append(surfaces, SurfaceDesktop)
	}
	return surfaces
}

func (s ScenarioEvaluation) SurfaceStatus(surface string) (bool, bool) {
	switch surface {
	case SurfaceBrowser:
		if !s.Scenario.BrowserRequired {
			return false, false
		}
		return s.BrowserOK, true
	case SurfaceIOS:
		if !s.Scenario.IOSRequired {
			return false, false
		}
		return s.IOSOK, true
	case SurfaceCurl:
		if !s.Scenario.CurlRequired {
			return false, false
		}
		return s.CurlOK, true
	case SurfaceCLI:
		if !s.Scenario.CLIRequired {
			return false, false
		}
		return s.CLIOK, true
	case SurfaceDesktop:
		if !s.Scenario.DesktopRequired {
			return false, false
		}
		return s.DesktopOK, true
	default:
		return false, false
	}
}

func (s ScenarioEvaluation) SurfaceIssues(surface string) []string {
	switch surface {
	case SurfaceBrowser:
		return append([]string(nil), s.BrowserIssues...)
	case SurfaceIOS:
		return append([]string(nil), s.IOSIssues...)
	case SurfaceCurl:
		return append([]string(nil), s.CurlIssues...)
	case SurfaceCLI:
		return append([]string(nil), s.CLIIssues...)
	case SurfaceDesktop:
		return append([]string(nil), s.DesktopIssues...)
	default:
		return nil
	}
}

// ScreenshotLogEntry is one step in the agent's verification walkthrough.
// The agent describes what it did, takes a screenshot, looks at the
// screenshot with its own vision, writes what it sees, and explains how
// what it sees compares to the scenario requirements.
type ScreenshotLogEntry struct {
	ID             string    `json:"id"`
	RunID          string    `json:"run_id"`
	ScenarioID     string    `json:"scenario_id"`
	SessionID      string    `json:"session_id"`
	Surface        string    `json:"surface"`
	Step           int       `json:"step"`
	Action         string    `json:"action"`
	ScreenshotPath string    `json:"screenshot_path"`
	SHA256         string    `json:"sha256"`
	Observation    string    `json:"observation"`
	Comparison     string    `json:"comparison"`
	CreatedAt      time.Time `json:"created_at"`
}

func normalizePlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "", PlatformWeb:
		return PlatformWeb
	case PlatformIOS:
		return PlatformIOS
	case PlatformCLI:
		return PlatformCLI
	case PlatformDesktop:
		return PlatformDesktop
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

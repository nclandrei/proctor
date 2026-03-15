package proctor

import "time"

const (
	SurfaceBrowser = "browser"
	SurfaceCurl    = "curl"

	TierWrappedCommand = 2
	TierRegisteredRun  = 3
	StatusInProgress   = "in_progress"
	StatusBlocked      = "blocked"
	StatusPassed       = "passed"
	AssertionPass      = "pass"
	AssertionFail      = "fail"
	ArtifactImage      = "image"
	ArtifactJSONReport = "json-report"
	ArtifactTranscript = "transcript"
	ArtifactHTML       = "html"
	ArtifactMarkdown   = "markdown"
	EdgeCategoryNA     = "na"
	EdgeCategoryScenar = "scenario"
)

var EdgeCaseCategories = []string{
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

type Run struct {
	ID                 string             `json:"id"`
	RepoSlug           string             `json:"repo_slug"`
	RepoRoot           string             `json:"repo_root"`
	Feature            string             `json:"feature"`
	BrowserURL         string             `json:"browser_url"`
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
	ID              string `json:"id"`
	Label           string `json:"label"`
	Kind            string `json:"kind"`
	Category        string `json:"category,omitempty"`
	BrowserRequired bool   `json:"browser_required"`
	CurlRequired    bool   `json:"curl_required"`
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
	Browser    *BrowserData `json:"browser,omitempty"`
	Curl       *CurlData    `json:"curl,omitempty"`
}

type Provenance struct {
	Mode       string   `json:"mode"`
	Tool       string   `json:"tool"`
	SessionID  string   `json:"session_id,omitempty"`
	Command    []string `json:"command,omitempty"`
	CWD        string   `json:"cwd"`
	RecordedBy string   `json:"recorded_by"`
}

type Assertion struct {
	Description string `json:"description"`
	Result      string `json:"result"`
}

type Artifact struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Source    string `json:"source,omitempty"`
	MediaType string `json:"media_type,omitempty"`
}

type BrowserData struct {
	URL       string `json:"url"`
	SessionID string `json:"session_id"`
	Tool      string `json:"tool"`
}

type CurlData struct {
	Command  []string `json:"command"`
	ExitCode int      `json:"exit_code"`
}

type StartOptions struct {
	Feature        string
	BrowserURL     string
	CurlMode       string
	CurlEndpoints  []string
	CurlSkipReason string
	HappyPath      string
	FailurePath    string
	EdgeCaseInputs []string
}

type BrowserRecordOptions struct {
	ScenarioID     string
	SessionID      string
	Tool           string
	Screenshots    map[string]string
	ReportPath     string
	PassAssertions []string
	FailAssertions []string
}

type CurlRecordOptions struct {
	ScenarioID     string
	Command        []string
	PassAssertions []string
	FailAssertions []string
}

type Evaluation struct {
	Complete            bool
	ScenarioEvaluations []ScenarioEvaluation
	GlobalMissing       []string
}

type ScenarioEvaluation struct {
	Scenario      Scenario
	BrowserOK     bool
	CurlOK        bool
	BrowserIssues []string
	CurlIssues    []string
}

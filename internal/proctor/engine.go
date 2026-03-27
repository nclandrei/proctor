package proctor

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func CreateRun(store *Store, cwd string, opts StartOptions) (Run, error) {
	platform := normalizePlatform(firstNonEmpty(opts.Platform, opts.Surface))
	switch platform {
	case PlatformWeb, PlatformIOS, PlatformCLI, PlatformDesktop:
	default:
		return Run{}, fmt.Errorf("--platform must be one of web, ios, cli, or desktop")
	}
	curlMode := strings.ToLower(strings.TrimSpace(opts.CurlMode))
	if strings.TrimSpace(opts.Feature) == "" {
		return Run{}, fmt.Errorf("--feature is required")
	}
	if strings.TrimSpace(opts.HappyPath) == "" {
		return Run{}, fmt.Errorf("--happy-path is required")
	}
	if strings.TrimSpace(opts.FailurePath) == "" {
		return Run{}, fmt.Errorf("--failure-path is required")
	}
	curlEndpoints := normalizedLines(opts.CurlEndpoints)

	switch platform {
	case PlatformWeb:
		switch curlMode {
		case CurlModeRequired, CurlModeScenario, CurlModeSkip:
		default:
			return Run{}, fmt.Errorf("--curl must be one of required, scenario, or skip")
		}
		if strings.TrimSpace(opts.BrowserURL) == "" {
			return Run{}, fmt.Errorf("--url is required when --platform web")
		}
		if strings.TrimSpace(opts.CLICommand) != "" {
			return Run{}, fmt.Errorf("--cli-command is only valid when --platform cli")
		}
		if strings.TrimSpace(opts.IOSScheme) != "" || strings.TrimSpace(opts.IOSBundleID) != "" || strings.TrimSpace(opts.IOSSimulator) != "" {
			return Run{}, fmt.Errorf("--ios-* flags are only valid when --platform ios")
		}
		if curlMode == CurlModeRequired && len(curlEndpoints) == 0 {
			return Run{}, fmt.Errorf("--curl-endpoint is required when --curl required")
		}
		if curlMode == CurlModeScenario && len(curlEndpoints) == 0 {
			return Run{}, fmt.Errorf("--curl-endpoint is required when --curl scenario")
		}
		if curlMode == CurlModeSkip && strings.TrimSpace(opts.CurlSkipReason) == "" {
			return Run{}, fmt.Errorf("--curl-skip-reason is required when --curl skip")
		}
	case PlatformIOS:
		switch curlMode {
		case CurlModeRequired, CurlModeScenario, CurlModeSkip:
		default:
			return Run{}, fmt.Errorf("--curl must be one of required, scenario, or skip")
		}
		if strings.TrimSpace(opts.IOSScheme) == "" {
			return Run{}, fmt.Errorf("--ios-scheme is required when --platform ios")
		}
		if strings.TrimSpace(opts.IOSBundleID) == "" {
			return Run{}, fmt.Errorf("--ios-bundle-id is required when --platform ios")
		}
		if strings.TrimSpace(opts.BrowserURL) != "" {
			return Run{}, fmt.Errorf("--url is only valid when --platform web")
		}
		if strings.TrimSpace(opts.CLICommand) != "" {
			return Run{}, fmt.Errorf("--cli-command is only valid when --platform cli")
		}
		if curlMode == CurlModeRequired && len(curlEndpoints) == 0 {
			return Run{}, fmt.Errorf("--curl-endpoint is required when --curl required")
		}
		if curlMode == CurlModeScenario && len(curlEndpoints) == 0 {
			return Run{}, fmt.Errorf("--curl-endpoint is required when --curl scenario")
		}
		if curlMode == CurlModeSkip && strings.TrimSpace(opts.CurlSkipReason) == "" {
			return Run{}, fmt.Errorf("--curl-skip-reason is required when --curl skip")
		}
	case PlatformCLI:
		if strings.TrimSpace(opts.CLICommand) == "" {
			return Run{}, fmt.Errorf("--cli-command is required when --platform cli")
		}
		if strings.TrimSpace(opts.BrowserURL) != "" || strings.TrimSpace(opts.IOSScheme) != "" || strings.TrimSpace(opts.IOSBundleID) != "" || strings.TrimSpace(opts.IOSSimulator) != "" {
			return Run{}, fmt.Errorf("--url and --ios-* flags are only valid on their matching platforms")
		}
		if strings.TrimSpace(opts.CurlMode) != "" || len(curlEndpoints) > 0 || strings.TrimSpace(opts.CurlSkipReason) != "" {
			return Run{}, fmt.Errorf("--curl, --curl-endpoint, and --curl-skip-reason are only valid when --platform web or --platform ios")
		}
	case PlatformDesktop:
		if strings.TrimSpace(opts.DesktopAppName) == "" {
			return Run{}, fmt.Errorf("--app-name is required when --platform desktop")
		}
		switch curlMode {
		case CurlModeRequired, CurlModeScenario, CurlModeSkip:
		default:
			return Run{}, fmt.Errorf("--curl must be one of required, scenario, or skip")
		}
		if strings.TrimSpace(opts.BrowserURL) != "" {
			return Run{}, fmt.Errorf("--url is only valid when --platform web")
		}
		if strings.TrimSpace(opts.CLICommand) != "" {
			return Run{}, fmt.Errorf("--cli-command is only valid when --platform cli")
		}
		if strings.TrimSpace(opts.IOSScheme) != "" || strings.TrimSpace(opts.IOSBundleID) != "" || strings.TrimSpace(opts.IOSSimulator) != "" {
			return Run{}, fmt.Errorf("--ios-* flags are only valid when --platform ios")
		}
		if curlMode == CurlModeRequired && len(curlEndpoints) == 0 {
			return Run{}, fmt.Errorf("--curl-endpoint is required when --curl required")
		}
		if curlMode == CurlModeScenario && len(curlEndpoints) == 0 {
			return Run{}, fmt.Errorf("--curl-endpoint is required when --curl scenario")
		}
		if curlMode == CurlModeSkip && strings.TrimSpace(opts.CurlSkipReason) == "" {
			return Run{}, fmt.Errorf("--curl-skip-reason is required when --curl skip")
		}
	}
	if err := validateEdgeCaseInputs(platform, opts.EdgeCaseInputs); err != nil {
		return Run{}, err
	}

	repoRoot := RepoRoot(cwd)
	repoSlug, err := RepoSlug(repoRoot)
	if err != nil {
		return Run{}, err
	}
	now := time.Now().UTC()
	browserRequired, iosRequired, cliRequired, desktopRequired := uiRequirementsForPlatform(platform)
	run := Run{
		ID:             newID("run"),
		RepoRoot:       repoRoot,
		RepoSlug:       repoSlug,
		Platform:       platform,
		Surface:        platform,
		Feature:        strings.TrimSpace(opts.Feature),
		BrowserURL:     strings.TrimSpace(opts.BrowserURL),
		CLICommand:     strings.TrimSpace(opts.CLICommand),
		Desktop:        DesktopApp{Name: strings.TrimSpace(opts.DesktopAppName), BundleID: strings.TrimSpace(opts.DesktopBundleID)},
		CurlMode:       curlMode,
		IOS:            IOSTarget{Scheme: strings.TrimSpace(opts.IOSScheme), BundleID: strings.TrimSpace(opts.IOSBundleID), Simulator: strings.TrimSpace(opts.IOSSimulator)},
		CurlRequired:   curlMode == CurlModeRequired,
		CurlEndpoints:  legacyRunCurlEndpoints(curlMode, curlEndpoints),
		CurlSkipReason: strings.TrimSpace(opts.CurlSkipReason),
		Status:         StatusInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	run.HappyPath = Scenario{
		ID:              "happy-path",
		Label:           strings.TrimSpace(opts.HappyPath),
		Kind:            "happy-path",
		BrowserRequired: browserRequired,
		IOSRequired:     iosRequired,
		CLIRequired:     cliRequired,
		DesktopRequired: desktopRequired,
		CurlRequired:    run.CurlRequired,
	}
	run.FailurePath = Scenario{
		ID:              "failure-path",
		Label:           strings.TrimSpace(opts.FailurePath),
		Kind:            "failure-path",
		BrowserRequired: browserRequired,
		IOSRequired:     iosRequired,
		CLIRequired:     cliRequired,
		DesktopRequired: desktopRequired,
		CurlRequired:    run.CurlRequired,
	}
	run.Scenarios = append(run.Scenarios, run.HappyPath, run.FailurePath)

	edgeCategories, edgeScenarios := parseEdgeCaseInputs(platform, opts.EdgeCaseInputs)
	run.EdgeCaseCategories = edgeCategories
	run.Scenarios = append(run.Scenarios, edgeScenarios...)
	if err := applyScenarioCurlPlan(&run, curlMode, curlEndpoints); err != nil {
		return Run{}, err
	}
	syncPrimaryScenarios(&run)

	runDir := store.RunDir(run)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		return Run{}, err
	}
	if err := store.SaveRun(run); err != nil {
		return Run{}, err
	}
	if err := store.SetActiveRun(run); err != nil {
		return Run{}, err
	}
	if err := writeReports(store, run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func RecordBrowser(store *Store, run Run, opts BrowserRecordOptions) error {
	if err := validateSurfaceForPlatform(SurfaceBrowser, run.Platform); err != nil {
		return err
	}
	scenario, ok := findScenario(run, opts.ScenarioID)
	if !ok {
		return fmt.Errorf("unknown scenario: %s", opts.ScenarioID)
	}
	if opts.SessionID == "" {
		return fmt.Errorf("browser evidence requires --session")
	}
	if opts.ReportPath == "" {
		return fmt.Errorf("browser evidence requires --report")
	}
	if len(opts.Screenshots) == 0 {
		return fmt.Errorf("browser evidence requires at least one --screenshot")
	}

	var artifacts []Artifact
	for label, source := range opts.Screenshots {
		artifact, err := store.CopyArtifact(run, SurfaceBrowser, scenario.ID, label, source)
		if err != nil {
			return err
		}
		artifact.Kind = ArtifactImage
		artifacts = append(artifacts, artifact)
	}
	reportArtifact, err := store.CopyArtifact(run, SurfaceBrowser, scenario.ID, "browser-report", opts.ReportPath)
	if err != nil {
		return err
	}
	reportArtifact.Kind = ArtifactJSONReport
	reportArtifact.MediaType = "application/json"
	artifacts = append(artifacts, reportArtifact)

	maxAge := opts.MaxScreenshotAge
	if maxAge == 0 {
		maxAge = DefaultMaxScreenshotAge
	}
	if err := validateScreenshotFreshness(artifacts, maxAge); err != nil {
		return err
	}
	if err := detectDuplicateScreenshots(store, run, scenario.ID, artifacts); err != nil {
		return err
	}

	reportData, err := ParseBrowserReport(opts.ReportPath)
	if err != nil {
		return err
	}
	assertions, err := EvaluateBrowserAssertions(opts.PassAssertions, opts.FailAssertions, reportData, artifacts)
	if err != nil {
		return err
	}
	if len(assertions) == 0 {
		return fmt.Errorf("browser evidence requires at least one assertion")
	}
	evidence := Evidence{
		ID:         newID("ev"),
		RunID:      run.ID,
		ScenarioID: scenario.ID,
		Surface:    SurfaceBrowser,
		Tier:       TierRegisteredRun,
		CreatedAt:  time.Now().UTC(),
		Title:      fmt.Sprintf("Browser verification for %s", scenario.Label),
		Provenance: Provenance{
			Mode:       "registered-session",
			Tool:       firstNonEmpty(opts.Tool, "agent-browser"),
			SessionID:  opts.SessionID,
			CWD:        run.RepoRoot,
			RecordedBy: "proctor",
		},
		Assertions: assertions,
		Artifacts:  artifacts,
		Browser: &BrowserData{
			URL:       run.BrowserURL,
			SessionID: opts.SessionID,
			Tool:      firstNonEmpty(opts.Tool, "agent-browser"),
			Desktop:   reportData.Desktop,
			Mobile:    reportData.Mobile,
		},
	}
	if err := store.AppendEvidence(run, evidence); err != nil {
		return err
	}
	if err := writeReports(store, run); err != nil {
		return err
	}
	return checkAssertionFailures(assertions)
}

func RecordIOS(store *Store, run Run, opts IOSRecordOptions) error {
	if err := validateSurfaceForPlatform(SurfaceIOS, run.Platform); err != nil {
		return err
	}
	scenario, ok := findScenario(run, opts.ScenarioID)
	if !ok {
		return fmt.Errorf("unknown scenario: %s", opts.ScenarioID)
	}
	if opts.SessionID == "" {
		return fmt.Errorf("ios evidence requires --session")
	}
	if opts.ReportPath == "" {
		return fmt.Errorf("ios evidence requires --report")
	}
	if len(opts.Screenshots) == 0 {
		return fmt.Errorf("ios evidence requires at least one --screenshot")
	}

	var artifacts []Artifact
	for label, source := range opts.Screenshots {
		artifact, err := store.CopyArtifact(run, SurfaceIOS, scenario.ID, label, source)
		if err != nil {
			return err
		}
		artifact.Kind = ArtifactImage
		artifacts = append(artifacts, artifact)
	}
	reportArtifact, err := store.CopyArtifact(run, SurfaceIOS, scenario.ID, "ios-report", opts.ReportPath)
	if err != nil {
		return err
	}
	reportArtifact.Kind = ArtifactJSONReport
	reportArtifact.MediaType = "application/json"
	artifacts = append(artifacts, reportArtifact)

	maxAge := opts.MaxScreenshotAge
	if maxAge == 0 {
		maxAge = DefaultMaxScreenshotAge
	}
	if err := validateScreenshotFreshness(artifacts, maxAge); err != nil {
		return err
	}
	if err := detectDuplicateScreenshots(store, run, scenario.ID, artifacts); err != nil {
		return err
	}

	reportData, err := ParseIOSReport(opts.ReportPath)
	if err != nil {
		return err
	}
	assertions, err := EvaluateIOSAssertions(opts.PassAssertions, opts.FailAssertions, reportData, artifacts)
	if err != nil {
		return err
	}
	if len(assertions) == 0 {
		return fmt.Errorf("ios evidence requires at least one assertion")
	}
	evidence := Evidence{
		ID:         newID("ev"),
		RunID:      run.ID,
		ScenarioID: scenario.ID,
		Surface:    SurfaceIOS,
		Tier:       TierRegisteredRun,
		CreatedAt:  time.Now().UTC(),
		Title:      fmt.Sprintf("iOS verification for %s", scenario.Label),
		Provenance: Provenance{
			Mode:       "registered-simulator-session",
			Tool:       firstNonEmpty(opts.Tool, "ios-simulator"),
			SessionID:  opts.SessionID,
			CWD:        run.RepoRoot,
			RecordedBy: "proctor",
		},
		Assertions: assertions,
		Artifacts:  artifacts,
		IOS:        &reportData,
	}
	if err := store.AppendEvidence(run, evidence); err != nil {
		return err
	}
	if err := writeReports(store, run); err != nil {
		return err
	}
	return checkAssertionFailures(assertions)
}

func RecordCurl(store *Store, run Run, opts CurlRecordOptions) error {
	if err := validateSurfaceForPlatform(SurfaceCurl, run.Platform); err != nil {
		return err
	}
	scenario, ok := findScenario(run, opts.ScenarioID)
	if !ok {
		return fmt.Errorf("unknown scenario: %s", opts.ScenarioID)
	}
	if len(opts.Command) == 0 {
		return fmt.Errorf("curl evidence requires a command after --")
	}
	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	cmd.Dir = run.RepoRoot
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return err
		}
	}

	content := []byte(fmt.Sprintf("$ %s\n\n%s%s", strings.Join(opts.Command, " "), stdout.String(), stderr.String()))
	transcript, err := store.WriteArtifact(run, SurfaceCurl, scenario.ID, "curl-transcript", ".txt", content)
	if err != nil {
		return err
	}
	transcript.Kind = ArtifactTranscript
	transcript.MediaType = "text/plain"
	statusCode, headers, body := ParseHTTPTranscript(stdout.String() + stderr.String())
	curlData := CurlData{
		Command:        opts.Command,
		ExitCode:       exitCode,
		ResponseStatus: statusCode,
		Headers:        headers,
		Body:           body,
	}
	assertions, err := EvaluateCurlAssertions(opts.PassAssertions, opts.FailAssertions, curlData)
	if err != nil {
		return err
	}
	if len(assertions) == 0 {
		return fmt.Errorf("curl evidence requires at least one assertion")
	}

	evidence := Evidence{
		ID:         newID("ev"),
		RunID:      run.ID,
		ScenarioID: scenario.ID,
		Surface:    SurfaceCurl,
		Tier:       TierWrappedCommand,
		CreatedAt:  time.Now().UTC(),
		Title:      fmt.Sprintf("curl verification for %s", scenario.Label),
		Provenance: Provenance{
			Mode:       "wrapped-command",
			Tool:       opts.Command[0],
			Command:    opts.Command,
			CWD:        run.RepoRoot,
			RecordedBy: "proctor",
		},
		Assertions: assertions,
		Artifacts:  []Artifact{transcript},
		Curl:       &curlData,
	}
	if err := store.AppendEvidence(run, evidence); err != nil {
		return err
	}
	if err := writeReports(store, run); err != nil {
		return err
	}
	return checkAssertionFailures(assertions)
}

func RecordCLI(store *Store, run Run, opts CLIRecordOptions) error {
	if err := validateSurfaceForPlatform(SurfaceCLI, run.Platform); err != nil {
		return err
	}
	scenario, ok := findScenario(run, opts.ScenarioID)
	if !ok {
		return fmt.Errorf("unknown scenario: %s", opts.ScenarioID)
	}
	if strings.TrimSpace(opts.SessionID) == "" {
		return fmt.Errorf("cli evidence requires --session")
	}
	if strings.TrimSpace(opts.Command) == "" {
		return fmt.Errorf("cli evidence requires --command")
	}
	if strings.TrimSpace(opts.TranscriptPath) == "" {
		return fmt.Errorf("cli evidence requires --transcript")
	}
	if len(opts.Screenshots) == 0 {
		return fmt.Errorf("cli evidence requires at least one --screenshot")
	}

	var artifacts []Artifact
	for label, source := range opts.Screenshots {
		artifact, err := store.CopyArtifact(run, SurfaceCLI, scenario.ID, label, source)
		if err != nil {
			return err
		}
		artifact.Kind = ArtifactImage
		artifacts = append(artifacts, artifact)
	}
	transcriptArtifact, err := store.CopyArtifact(run, SurfaceCLI, scenario.ID, "cli-transcript", opts.TranscriptPath)
	if err != nil {
		return err
	}
	transcriptArtifact.Kind = ArtifactTranscript
	transcriptArtifact.MediaType = "text/plain"
	artifacts = append(artifacts, transcriptArtifact)

	maxAge := opts.MaxScreenshotAge
	if maxAge == 0 {
		maxAge = DefaultMaxScreenshotAge
	}
	if err := validateScreenshotFreshness(artifacts, maxAge); err != nil {
		return err
	}
	if err := detectDuplicateScreenshots(store, run, scenario.ID, artifacts); err != nil {
		return err
	}

	transcriptBytes, err := os.ReadFile(opts.TranscriptPath)
	if err != nil {
		return err
	}
	transcript := normalizeTranscript(string(transcriptBytes))
	assertions, err := EvaluateCLIAssertions(opts.PassAssertions, opts.FailAssertions, CLIData{
		Command:           strings.TrimSpace(opts.Command),
		SessionID:         strings.TrimSpace(opts.SessionID),
		Tool:              firstNonEmpty(opts.Tool, "terminal-session"),
		ExitCode:          opts.ExitCode,
		TranscriptPreview: transcriptPreview(transcript),
	}, transcript, artifacts)
	if err != nil {
		return err
	}
	if len(assertions) == 0 {
		return fmt.Errorf("cli evidence requires at least one assertion")
	}

	evidence := Evidence{
		ID:         newID("ev"),
		RunID:      run.ID,
		ScenarioID: scenario.ID,
		Surface:    SurfaceCLI,
		Tier:       TierRegisteredRun,
		CreatedAt:  time.Now().UTC(),
		Title:      fmt.Sprintf("CLI verification for %s", scenario.Label),
		Provenance: Provenance{
			Mode:       "registered-session",
			Tool:       firstNonEmpty(opts.Tool, "terminal-session"),
			Command:    []string{strings.TrimSpace(opts.Command)},
			SessionID:  strings.TrimSpace(opts.SessionID),
			CWD:        run.RepoRoot,
			RecordedBy: "proctor",
		},
		Assertions: assertions,
		Artifacts:  artifacts,
		CLI: &CLIData{
			Command:           strings.TrimSpace(opts.Command),
			SessionID:         strings.TrimSpace(opts.SessionID),
			Tool:              firstNonEmpty(opts.Tool, "terminal-session"),
			ExitCode:          opts.ExitCode,
			TranscriptPreview: transcriptPreview(transcript),
		},
	}
	if err := store.AppendEvidence(run, evidence); err != nil {
		return err
	}
	if err := writeReports(store, run); err != nil {
		return err
	}
	return checkAssertionFailures(assertions)
}

func RecordDesktop(store *Store, run Run, opts DesktopRecordOptions) error {
	if err := validateSurfaceForPlatform(SurfaceDesktop, run.Platform); err != nil {
		return err
	}
	scenario, ok := findScenario(run, opts.ScenarioID)
	if !ok {
		return fmt.Errorf("unknown scenario: %s", opts.ScenarioID)
	}
	if opts.SessionID == "" {
		return fmt.Errorf("desktop evidence requires --session")
	}
	if opts.ReportPath == "" {
		return fmt.Errorf("desktop evidence requires --report")
	}
	if len(opts.Screenshots) == 0 {
		return fmt.Errorf("desktop evidence requires at least one --screenshot")
	}

	var artifacts []Artifact
	for label, source := range opts.Screenshots {
		artifact, err := store.CopyArtifact(run, SurfaceDesktop, scenario.ID, label, source)
		if err != nil {
			return err
		}
		artifact.Kind = ArtifactImage
		artifacts = append(artifacts, artifact)
	}
	reportArtifact, err := store.CopyArtifact(run, SurfaceDesktop, scenario.ID, "desktop-report", opts.ReportPath)
	if err != nil {
		return err
	}
	reportArtifact.Kind = ArtifactJSONReport
	reportArtifact.MediaType = "application/json"
	artifacts = append(artifacts, reportArtifact)

	maxAge := opts.MaxScreenshotAge
	if maxAge == 0 {
		maxAge = DefaultMaxScreenshotAge
	}
	if err := validateScreenshotFreshness(artifacts, maxAge); err != nil {
		return err
	}
	if err := detectDuplicateScreenshots(store, run, scenario.ID, artifacts); err != nil {
		return err
	}

	reportData, err := ParseDesktopReport(opts.ReportPath)
	if err != nil {
		return err
	}
	reportData.SessionID = opts.SessionID
	reportData.Tool = firstNonEmpty(opts.Tool, "peekaboo")
	assertions, err := EvaluateDesktopAssertions(opts.PassAssertions, opts.FailAssertions, reportData, artifacts)
	if err != nil {
		return err
	}
	if len(assertions) == 0 {
		return fmt.Errorf("desktop evidence requires at least one assertion")
	}
	evidence := Evidence{
		ID:         newID("ev"),
		RunID:      run.ID,
		ScenarioID: scenario.ID,
		Surface:    SurfaceDesktop,
		Tier:       TierRegisteredRun,
		CreatedAt:  time.Now().UTC(),
		Title:      fmt.Sprintf("Desktop verification for %s", scenario.Label),
		Provenance: Provenance{
			Mode:       "registered-session",
			Tool:       firstNonEmpty(opts.Tool, "peekaboo"),
			SessionID:  opts.SessionID,
			CWD:        run.RepoRoot,
			RecordedBy: "proctor",
		},
		Assertions: assertions,
		Artifacts:  artifacts,
		Desktop:    &reportData,
	}
	if err := store.AppendEvidence(run, evidence); err != nil {
		return err
	}
	if err := writeReports(store, run); err != nil {
		return err
	}
	return checkAssertionFailures(assertions)
}

func Evaluate(store *Store, run Run) (Evaluation, error) {
	evidence, err := store.LoadEvidence(run)
	if err != nil {
		return Evaluation{}, err
	}

	eval := Evaluation{Complete: true}
	platform := normalizePlatform(run.Platform)
	hasDesktop := true
	hasMobile := true
	hasIOSScreenshot := true
	hasDesktopScreenshot := true
	switch platform {
	case PlatformIOS:
		hasIOSScreenshot = iosVisualCoverage(store, run, evidence)
	case PlatformWeb:
		hasDesktop, hasMobile = browserVisualCoverage(store, run, evidence)
	case PlatformDesktop:
		hasDesktopScreenshot = desktopVisualCoverage(store, run, evidence)
	}

	for _, scenario := range run.Scenarios {
		scenarioEval := ScenarioEvaluation{Scenario: scenario}
		browserEvidence := selectEvidenceForScenario(evidence, scenario.ID, SurfaceBrowser)
		iosEvidence := selectEvidenceForScenario(evidence, scenario.ID, SurfaceIOS)
		curlEvidence := selectEvidenceForScenario(evidence, scenario.ID, SurfaceCurl)
		cliEvidence := selectEvidenceForScenario(evidence, scenario.ID, SurfaceCLI)

		if scenario.BrowserRequired {
			scenarioEval.BrowserOK, scenarioEval.BrowserIssues = validateBrowserEvidence(store, run, scenario, browserEvidence)
			if !scenarioEval.BrowserOK {
				eval.Complete = false
			}
		}
		if scenario.IOSRequired {
			scenarioEval.IOSOK, scenarioEval.IOSIssues = validateIOSEvidence(store, run, iosEvidence)
			if !scenarioEval.IOSOK {
				eval.Complete = false
			}
		}

		if scenario.CurlRequired {
			scenarioEval.CurlOK, scenarioEval.CurlIssues = validateCurlEvidence(store, run, scenario, curlEvidence)
			if !scenarioEval.CurlOK {
				eval.Complete = false
			}
		}
		if scenario.CLIRequired {
			scenarioEval.CLIOK, scenarioEval.CLIIssues = validateCLIEvidence(store, run, cliEvidence)
			if !scenarioEval.CLIOK {
				eval.Complete = false
			}
		}
		if scenario.DesktopRequired {
			desktopEvidence := selectEvidenceForScenario(evidence, scenario.ID, SurfaceDesktop)
			scenarioEval.DesktopOK, scenarioEval.DesktopIssues = validateDesktopEvidence(store, run, desktopEvidence)
			if !scenarioEval.DesktopOK {
				eval.Complete = false
			}
		}

		eval.ScenarioEvaluations = append(eval.ScenarioEvaluations, scenarioEval)
	}

	switch platform {
	case PlatformIOS:
		if !hasIOSScreenshot {
			eval.Complete = false
			eval.GlobalMissing = append(eval.GlobalMissing, "at least one iOS screenshot")
		}
	case PlatformWeb:
		if !hasDesktop {
			eval.Complete = false
			eval.GlobalMissing = append(eval.GlobalMissing, "at least one desktop screenshot")
		}
		if !hasMobile {
			eval.Complete = false
			eval.GlobalMissing = append(eval.GlobalMissing, "at least one mobile screenshot")
		}
	case PlatformDesktop:
		if !hasDesktopScreenshot {
			eval.Complete = false
			eval.GlobalMissing = append(eval.GlobalMissing, "at least one desktop app screenshot")
		}
	}

	return eval, nil
}

func browserVisualCoverage(store *Store, run Run, evidence []Evidence) (bool, bool) {
	hasDesktop := false
	hasMobile := false
	for _, item := range evidence {
		if item.Surface != SurfaceBrowser || item.Tier < TierRegisteredRun {
			continue
		}
		for _, artifact := range item.Artifacts {
			if artifact.Kind != ArtifactImage {
				continue
			}
			if err := store.VerifyArtifactHash(run, artifact); err != nil {
				continue
			}
			label := strings.ToLower(artifact.Label)
			if strings.Contains(label, "desktop") {
				hasDesktop = true
			}
			if strings.Contains(label, "mobile") {
				hasMobile = true
			}
		}
	}
	return hasDesktop, hasMobile
}

func iosVisualCoverage(store *Store, run Run, evidence []Evidence) bool {
	for _, item := range evidence {
		if item.Surface != SurfaceIOS || item.Tier < TierRegisteredRun {
			continue
		}
		for _, artifact := range item.Artifacts {
			if artifact.Kind != ArtifactImage {
				continue
			}
			if err := store.VerifyArtifactHash(run, artifact); err == nil {
				return true
			}
		}
	}
	return false
}

func desktopVisualCoverage(store *Store, run Run, evidence []Evidence) bool {
	for _, item := range evidence {
		if item.Surface != SurfaceDesktop || item.Tier < TierRegisteredRun {
			continue
		}
		for _, artifact := range item.Artifacts {
			if artifact.Kind != ArtifactImage {
				continue
			}
			if err := store.VerifyArtifactHash(run, artifact); err == nil {
				return true
			}
		}
	}
	return false
}

func validateDesktopEvidence(store *Store, run Run, items []Evidence) (bool, []string) {
	if len(items) == 0 {
		return false, []string{"missing desktop evidence"}
	}
	for _, item := range items {
		issues := desktopEvidenceIssues(store, run, item)
		if len(issues) == 0 {
			return true, nil
		}
	}
	var issues []string
	for _, item := range items {
		issues = append(issues, desktopEvidenceIssues(store, run, item)...)
	}
	return false, dedupeStrings(issues)
}

func desktopEvidenceIssues(store *Store, run Run, item Evidence) []string {
	var issues []string
	if item.Tier < TierRegisteredRun {
		issues = append(issues, fmt.Sprintf("desktop evidence tier %d is below required tier %d", item.Tier, TierRegisteredRun))
	}
	if item.Provenance.SessionID == "" {
		issues = append(issues, "desktop evidence is missing a registered session id")
	}
	issues = append(issues, desktopReportStructureIssues(item)...)
	issues = append(issues, assertionIssues(item.Assertions, "desktop")...)

	hasImage := false
	hasReport := false
	for _, artifact := range item.Artifacts {
		if err := store.VerifyArtifactHash(run, artifact); err != nil {
			issues = append(issues, fmt.Sprintf("artifact hash mismatch for %s", artifact.Label))
			continue
		}
		switch artifact.Kind {
		case ArtifactImage:
			hasImage = true
		case ArtifactJSONReport:
			hasReport = true
		}
	}
	if !hasImage {
		issues = append(issues, "desktop evidence is missing a screenshot")
	}
	if !hasReport {
		issues = append(issues, "desktop evidence is missing a desktop report")
	}
	return dedupeStrings(issues)
}

func desktopReportStructureIssues(item Evidence) []string {
	if item.Desktop == nil {
		return []string{"desktop evidence is missing parsed desktop report data"}
	}

	var issues []string
	if strings.TrimSpace(item.Desktop.AppName) == "" {
		issues = append(issues, "desktop report is missing an app name")
	}
	return issues
}

func CompleteRun(store *Store, run Run) (Evaluation, error) {
	eval, err := Evaluate(store, run)
	if err != nil {
		return Evaluation{}, err
	}
	if eval.Complete {
		run.Status = StatusPassed
	} else {
		run.Status = StatusBlocked
	}
	if err := store.SaveRun(run); err != nil {
		return Evaluation{}, err
	}
	if err := writeReports(store, run); err != nil {
		return Evaluation{}, err
	}
	return eval, nil
}

func WriteReports(store *Store, run Run) error {
	return writeReports(store, run)
}

func writeReports(store *Store, run Run) error {
	eval, err := Evaluate(store, run)
	if err != nil {
		return err
	}
	evidence, err := store.LoadEvidence(run)
	if err != nil {
		return err
	}
	markdown, html, err := RenderReports(run, store.RunDir(run), eval, evidence)
	if err != nil {
		return err
	}
	mdArtifact, err := store.WriteArtifact(run, "reports", "summary", "contract", ".md", []byte(markdown))
	if err != nil {
		return err
	}
	_ = mdArtifact
	if err := os.WriteFile(filepath.Join(store.RunDir(run), "contract.md"), []byte(markdown), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(store.RunDir(run), "report.html"), []byte(html), 0o644); err != nil {
		return err
	}
	return nil
}

func normalizedLines(values []string) []string {
	var normalized []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func legacyRunCurlEndpoints(curlMode string, curlEndpoints []string) []string {
	if curlMode != CurlModeRequired {
		return nil
	}
	return append([]string(nil), curlEndpoints...)
}

func applyScenarioCurlPlan(run *Run, curlMode string, curlEndpoints []string) error {
	switch curlMode {
	case CurlModeRequired:
		for _, scenarioID := range []string{"happy-path", "failure-path"} {
			setScenarioCurlPlan(run, scenarioID, curlEndpoints)
		}
	case CurlModeScenario:
		for _, spec := range curlEndpoints {
			ref, endpoints, err := parseScenarioCurlSpec(spec)
			if err != nil {
				return err
			}
			scenarioID, err := resolveScenarioRef(*run, ref)
			if err != nil {
				return err
			}
			setScenarioCurlPlan(run, scenarioID, endpoints)
		}
	case CurlModeSkip:
		return nil
	}
	return nil
}

func parseScenarioCurlSpec(value string) (string, []string, error) {
	ref, rawEndpoints, ok := strings.Cut(value, "=")
	if !ok {
		return "", nil, fmt.Errorf("invalid --curl-endpoint for --curl scenario: %s", value)
	}
	ref = strings.TrimSpace(ref)
	endpoints := splitAndNormalize(rawEndpoints, ";")
	if ref == "" || len(endpoints) == 0 {
		return "", nil, fmt.Errorf("invalid --curl-endpoint for --curl scenario: %s", value)
	}
	return ref, endpoints, nil
}

func resolveScenarioRef(run Run, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("curl scenario reference is required")
	}

	if scenarioID, ok := matchScenarioRef(run.Scenarios, func(s Scenario) bool {
		return strings.EqualFold(ref, s.ID) || (s.Kind != "edge-case" && strings.EqualFold(ref, s.Kind))
	}); ok {
		return scenarioID, nil
	}

	if category, label, ok := strings.Cut(ref, ":"); ok {
		if scenarioID, matchErr := matchUniqueScenario(run.Scenarios, func(s Scenario) bool {
			return s.Kind == "edge-case" &&
				strings.EqualFold(strings.TrimSpace(category), s.Category) &&
				strings.EqualFold(strings.TrimSpace(label), s.Label)
		}); matchErr == nil {
			return scenarioID, nil
		} else if matchErr != errNoScenarioMatch {
			return "", matchErr
		}
	}

	if scenarioID, matchErr := matchUniqueScenario(run.Scenarios, func(s Scenario) bool {
		return strings.EqualFold(ref, s.Label)
	}); matchErr == nil {
		return scenarioID, nil
	} else if matchErr != errNoScenarioMatch {
		return "", matchErr
	}

	if scenarioID, matchErr := matchUniqueScenario(run.Scenarios, func(s Scenario) bool {
		return s.Kind == "edge-case" && strings.EqualFold(ref, s.Category)
	}); matchErr == nil {
		return scenarioID, nil
	} else if matchErr != errNoScenarioMatch {
		return "", matchErr
	}

	return "", fmt.Errorf("unknown curl scenario reference: %s", ref)
}

var errNoScenarioMatch = fmt.Errorf("no scenario match")

func matchScenarioRef(scenarios []Scenario, match func(Scenario) bool) (string, bool) {
	for _, scenario := range scenarios {
		if match(scenario) {
			return scenario.ID, true
		}
	}
	return "", false
}

func matchUniqueScenario(scenarios []Scenario, match func(Scenario) bool) (string, error) {
	var matches []string
	for _, scenario := range scenarios {
		if match(scenario) {
			matches = append(matches, scenario.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", errNoScenarioMatch
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous curl scenario reference; use the scenario id or category:label")
	}
}

func setScenarioCurlPlan(run *Run, scenarioID string, endpoints []string) {
	for idx := range run.Scenarios {
		if run.Scenarios[idx].ID != scenarioID {
			continue
		}
		run.Scenarios[idx].CurlRequired = true
		run.Scenarios[idx].CurlEndpoints = append(run.Scenarios[idx].CurlEndpoints, endpoints...)
		run.Scenarios[idx].CurlEndpoints = dedupeStrings(run.Scenarios[idx].CurlEndpoints)
		break
	}
}

func syncPrimaryScenarios(run *Run) {
	if scenario, ok := findScenario(*run, "happy-path"); ok {
		run.HappyPath = scenario
	}
	if scenario, ok := findScenario(*run, "failure-path"); ok {
		run.FailurePath = scenario
	}
}

func normalizeRunCurlPlan(run *Run) {
	if !hasScenarioCurlRequirements(*run) && run.CurlRequired {
		for _, scenarioID := range []string{"happy-path", "failure-path"} {
			setScenarioCurlPlan(run, scenarioID, run.CurlEndpoints)
		}
	}
	if strings.TrimSpace(run.CurlMode) == "" {
		switch {
		case run.CurlRequired:
			run.CurlMode = CurlModeRequired
		case hasScenarioCurlRequirements(*run):
			run.CurlMode = CurlModeScenario
		default:
			run.CurlMode = CurlModeSkip
		}
	}
	syncPrimaryScenarios(run)
}

func hasScenarioCurlRequirements(run Run) bool {
	for _, scenario := range run.Scenarios {
		if scenario.CurlRequired || len(scenario.CurlEndpoints) > 0 {
			return true
		}
	}
	return false
}

func splitAndNormalize(value, sep string) []string {
	var normalized []string
	for _, part := range strings.Split(value, sep) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalized = append(normalized, part)
	}
	return normalized
}

func validateEdgeCaseInputs(platform string, inputs []string) error {
	responses := edgeCaseResponseMap(inputs)
	for _, category := range EdgeCaseCategoriesForPlatform(platform) {
		response, ok := responses[strings.ToLower(category)]
		if !ok || strings.TrimSpace(response) == "" {
			return fmt.Errorf("missing required edge-case coverage for %q", category)
		}

		if reason, isNA := edgeCaseNAReason(response); isNA {
			if reason == "" {
				return fmt.Errorf("edge-case %q must use %q", category, "N/A: reason")
			}
			continue
		}

		if len(splitAndNormalize(response, ";")) == 0 {
			return fmt.Errorf("edge-case %q must list one or more concrete scenarios or use %q", category, "N/A: reason")
		}
	}
	return nil
}

func edgeCaseResponseMap(inputs []string) map[string]string {
	responses := map[string]string{}
	for _, input := range inputs {
		category, response, ok := parseEdgeCaseInput(input)
		if !ok {
			continue
		}
		key := strings.ToLower(category)
		if _, exists := responses[key]; exists {
			continue
		}
		responses[key] = response
	}
	return responses
}

func parseEdgeCaseInput(input string) (string, string, bool) {
	if key, value, ok := strings.Cut(input, "="); ok {
		return strings.TrimSpace(key), strings.TrimSpace(value), true
	}
	if key, value, ok := strings.Cut(input, ":"); ok {
		return strings.TrimSpace(key), strings.TrimSpace(value), true
	}
	return "", "", false
}

func edgeCaseNAReason(response string) (string, bool) {
	response = strings.TrimSpace(response)
	if len(response) < 3 || !strings.EqualFold(response[:3], "n/a") {
		return "", false
	}

	remainder := strings.TrimSpace(response[3:])
	if !strings.HasPrefix(remainder, ":") {
		return "", true
	}

	return strings.TrimSpace(strings.TrimPrefix(remainder, ":")), true
}

func parseEdgeCaseInputs(platform string, inputs []string) ([]EdgeCaseCategory, []Scenario) {
	var categories []EdgeCaseCategory
	var scenarios []Scenario
	browserRequired, iosRequired, cliRequired, desktopRequired := uiRequirementsForPlatform(platform)
	responses := edgeCaseResponseMap(inputs)
	for _, category := range EdgeCaseCategoriesForPlatform(platform) {
		entry := EdgeCaseCategory{Category: category}
		response := responses[strings.ToLower(category)]
		if strings.EqualFold(response, "") {
			entry.Status = EdgeCategoryNA
			entry.Reason = "No answer recorded"
			categories = append(categories, entry)
			continue
		}
		if reason, isNA := edgeCaseNAReason(response); isNA {
			entry.Status = EdgeCategoryNA
			entry.Reason = reason
			if entry.Reason == "" {
				entry.Reason = "Marked not applicable"
			}
			categories = append(categories, entry)
			continue
		}
		entry.Status = EdgeCategoryScenar
		parts := strings.Split(response, ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			scenario := Scenario{
				ID:              slugify(category + "-" + part),
				Label:           part,
				Kind:            "edge-case",
				Category:        category,
				BrowserRequired: browserRequired,
				IOSRequired:     iosRequired,
				CLIRequired:     cliRequired,
				DesktopRequired: desktopRequired,
				CurlRequired:    false,
			}
			entry.ScenarioIDs = append(entry.ScenarioIDs, scenario.ID)
			scenarios = append(scenarios, scenario)
		}
		categories = append(categories, entry)
	}
	return categories, scenarios
}

func findScenario(run Run, scenarioID string) (Scenario, bool) {
	for _, scenario := range run.Scenarios {
		if scenario.ID == scenarioID {
			return scenario, true
		}
	}
	return Scenario{}, false
}

func selectEvidenceForScenario(items []Evidence, scenarioID, surface string) []Evidence {
	var selected []Evidence
	for _, item := range items {
		if item.ScenarioID == scenarioID && item.Surface == surface {
			selected = append(selected, item)
		}
	}
	return selected
}

func validateBrowserEvidence(store *Store, run Run, scenario Scenario, items []Evidence) (bool, []string) {
	if len(items) == 0 {
		return false, []string{"missing browser evidence"}
	}
	for _, item := range items {
		issues := browserEvidenceIssues(store, run, scenario, item)
		if len(issues) == 0 {
			return true, nil
		}
	}
	var issues []string
	for _, item := range items {
		issues = append(issues, browserEvidenceIssues(store, run, scenario, item)...)
	}
	return false, dedupeStrings(issues)
}

func validateIOSEvidence(store *Store, run Run, items []Evidence) (bool, []string) {
	if len(items) == 0 {
		return false, []string{"missing ios evidence"}
	}
	for _, item := range items {
		issues := iosEvidenceIssues(store, run, item)
		if len(issues) == 0 {
			return true, nil
		}
	}
	var issues []string
	for _, item := range items {
		issues = append(issues, iosEvidenceIssues(store, run, item)...)
	}
	return false, dedupeStrings(issues)
}

func validateCurlEvidence(store *Store, run Run, scenario Scenario, items []Evidence) (bool, []string) {
	if len(items) == 0 {
		return false, []string{"missing curl evidence"}
	}
	for _, item := range items {
		issues := curlEvidenceIssues(store, run, scenario, item)
		if len(issues) == 0 {
			return true, nil
		}
	}
	var issues []string
	for _, item := range items {
		issues = append(issues, curlEvidenceIssues(store, run, scenario, item)...)
	}
	return false, dedupeStrings(issues)
}

func validateCLIEvidence(store *Store, run Run, items []Evidence) (bool, []string) {
	if len(items) == 0 {
		return false, []string{"missing cli evidence"}
	}
	for _, item := range items {
		issues := cliEvidenceIssues(store, run, item)
		if len(issues) == 0 {
			return true, nil
		}
	}
	var issues []string
	for _, item := range items {
		issues = append(issues, cliEvidenceIssues(store, run, item)...)
	}
	return false, dedupeStrings(issues)
}

func assertionsPass(assertions []Assertion) bool {
	if len(assertions) == 0 {
		return false
	}
	passCount := 0
	for _, assertion := range assertions {
		if assertion.Result == AssertionFail {
			return false
		}
		if assertion.Result == AssertionPass {
			passCount++
		}
	}
	return passCount > 0
}

func browserEvidenceIssues(store *Store, run Run, scenario Scenario, item Evidence) []string {
	var issues []string
	if item.Tier < TierRegisteredRun {
		issues = append(issues, fmt.Sprintf("browser evidence tier %d is below required tier %d", item.Tier, TierRegisteredRun))
	}
	if item.Provenance.SessionID == "" {
		issues = append(issues, "browser evidence is missing a registered session id")
	}
	issues = append(issues, browserReportStructureIssues(item)...)
	issues = append(issues, scenarioSpecificBrowserIssues(scenario, item)...)
	issues = append(issues, assertionIssues(item.Assertions, "browser")...)

	hasImage := false
	hasReport := false
	for _, artifact := range item.Artifacts {
		if err := store.VerifyArtifactHash(run, artifact); err != nil {
			issues = append(issues, fmt.Sprintf("artifact hash mismatch for %s", artifact.Label))
			continue
		}
		switch artifact.Kind {
		case ArtifactImage:
			hasImage = true
		case ArtifactJSONReport:
			hasReport = true
		}
	}
	if !hasImage {
		issues = append(issues, "browser evidence is missing a screenshot")
	}
	if !hasReport {
		issues = append(issues, "browser evidence is missing a browser report")
	}
	return dedupeStrings(issues)
}

func iosEvidenceIssues(store *Store, run Run, item Evidence) []string {
	var issues []string
	if item.Tier < TierRegisteredRun {
		issues = append(issues, fmt.Sprintf("ios evidence tier %d is below required tier %d", item.Tier, TierRegisteredRun))
	}
	if item.Provenance.SessionID == "" {
		issues = append(issues, "ios evidence is missing a simulator session id")
	}
	issues = append(issues, iosReportStructureIssues(item)...)
	issues = append(issues, assertionIssues(item.Assertions, "ios")...)

	hasImage := false
	hasReport := false
	for _, artifact := range item.Artifacts {
		if err := store.VerifyArtifactHash(run, artifact); err != nil {
			issues = append(issues, fmt.Sprintf("artifact hash mismatch for %s", artifact.Label))
			continue
		}
		switch artifact.Kind {
		case ArtifactImage:
			hasImage = true
		case ArtifactJSONReport:
			hasReport = true
		}
	}
	if !hasImage {
		issues = append(issues, "ios evidence is missing a screenshot")
	}
	if !hasReport {
		issues = append(issues, "ios evidence is missing an ios report")
	}
	return dedupeStrings(issues)
}

func scenarioSpecificBrowserIssues(scenario Scenario, item Evidence) []string {
	if !scenarioNeedsMobileProof(scenario) {
		return nil
	}

	var issues []string
	if !hasScreenshotLabel(item.Artifacts, "mobile") {
		issues = append(issues, "mobile or responsive behavior scenarios require a mobile screenshot")
	}
	if item.Browser == nil || item.Browser.Mobile == nil {
		issues = append(issues, "mobile or responsive behavior scenarios require mobile browser results")
		return issues
	}
	if strings.TrimSpace(item.Browser.Mobile.FinalURL) == "" {
		issues = append(issues, "mobile or responsive behavior scenarios require a mobile final URL")
	}
	return issues
}

func scenarioNeedsMobileProof(scenario Scenario) bool {
	return strings.EqualFold(strings.TrimSpace(scenario.Category), "mobile or responsive behavior")
}

func browserReportStructureIssues(item Evidence) []string {
	if item.Browser == nil {
		return []string{"browser evidence is missing parsed browser report data"}
	}

	var issues []string
	if strings.TrimSpace(item.Browser.Desktop.FinalURL) == "" {
		issues = append(issues, "browser report is missing a desktop final URL")
	}
	if hasScreenshotLabel(item.Artifacts, "mobile") {
		if item.Browser.Mobile == nil {
			issues = append(issues, "browser report is missing mobile results for attached mobile screenshot")
		} else if strings.TrimSpace(item.Browser.Mobile.FinalURL) == "" {
			issues = append(issues, "browser report is missing a mobile final URL")
		}
	}
	return issues
}

func iosReportStructureIssues(item Evidence) []string {
	if item.IOS == nil {
		return []string{"ios evidence is missing parsed ios report data"}
	}

	var issues []string
	if strings.TrimSpace(item.IOS.BundleID) == "" {
		issues = append(issues, "ios report is missing an app bundle id")
	}
	if strings.TrimSpace(item.IOS.Screen) == "" {
		issues = append(issues, "ios report is missing a screen description")
	}
	if strings.TrimSpace(item.IOS.Simulator.Name) == "" {
		issues = append(issues, "ios report is missing a simulator name")
	}
	return issues
}

func curlEvidenceIssues(store *Store, run Run, scenario Scenario, item Evidence) []string {
	var issues []string
	if item.Tier < TierWrappedCommand {
		issues = append(issues, fmt.Sprintf("curl evidence tier %d is below required tier %d", item.Tier, TierWrappedCommand))
	}
	if len(item.Provenance.Command) == 0 {
		issues = append(issues, "curl evidence is missing a wrapped command")
	}
	if item.Curl == nil {
		issues = append(issues, "curl evidence is missing parsed HTTP response data")
	} else if item.Curl.ResponseStatus == 0 {
		issues = append(issues, "curl evidence is missing a real HTTP response")
	}
	issues = append(issues, curlEndpointContractIssues(scenario, item)...)
	issues = append(issues, assertionIssues(item.Assertions, "curl")...)

	hasTranscript := false
	for _, artifact := range item.Artifacts {
		if err := store.VerifyArtifactHash(run, artifact); err != nil {
			issues = append(issues, fmt.Sprintf("artifact hash mismatch for %s", artifact.Label))
			continue
		}
		if artifact.Kind == ArtifactTranscript {
			hasTranscript = true
		}
	}
	if !hasTranscript {
		issues = append(issues, "curl evidence is missing a transcript")
	}
	return dedupeStrings(issues)
}

func curlEndpointContractIssues(scenario Scenario, item Evidence) []string {
	if len(scenario.CurlEndpoints) == 0 {
		return nil
	}

	command := item.Provenance.Command
	if len(command) == 0 && item.Curl != nil {
		command = item.Curl.Command
	}
	method, path, ok := inferHTTPCommandTarget(command)
	if !ok {
		return []string{"curl evidence command does not expose an HTTP method and URL required to match the scenario endpoint contract"}
	}

	for _, endpoint := range scenario.CurlEndpoints {
		expectedMethod, expectedPath, ok := parseCurlEndpointContract(endpoint)
		if !ok {
			continue
		}
		if method == expectedMethod && path == expectedPath {
			return nil
		}
	}

	return []string{
		fmt.Sprintf(
			"curl evidence command %s %s does not match the scenario endpoint contract (%s)",
			method,
			path,
			strings.Join(scenario.CurlEndpoints, ", "),
		),
	}
}

func parseCurlEndpointContract(endpoint string) (string, string, bool) {
	fields := strings.Fields(strings.TrimSpace(endpoint))
	if len(fields) < 2 {
		return "", "", false
	}
	method := strings.ToUpper(fields[0])
	path, ok := normalizeContractPath(strings.Join(fields[1:], " "))
	if !ok {
		return "", "", false
	}
	return method, path, true
}

func inferHTTPCommandTarget(command []string) (string, string, bool) {
	tokens := flattenCommandTokens(command)
	if len(tokens) == 0 {
		return "", "", false
	}

	method := ""
	urlValue := ""
	hasBodyFlag := false
	for idx := 0; idx < len(tokens); idx++ {
		token := normalizeCommandToken(tokens[idx])
		if token == "" {
			continue
		}

		lowerToken := strings.ToLower(token)
		switch lowerToken {
		case "-x", "--request", "--method":
			if idx+1 < len(tokens) {
				if candidate := normalizeCommandToken(tokens[idx+1]); isHTTPMethod(candidate) {
					method = strings.ToUpper(candidate)
				}
			}
		case "-d", "--data", "--data-raw", "--data-binary", "--data-urlencode":
			hasBodyFlag = true
		default:
			if token == "-F" || lowerToken == "--form" {
				hasBodyFlag = true
				continue
			}
			if method == "" && isHTTPMethod(token) {
				method = strings.ToUpper(token)
				continue
			}
			if requestPath, ok := extractRequestPath(token); ok {
				urlValue = requestPath
			}
		}
	}

	if urlValue == "" {
		return "", "", false
	}
	if method == "" {
		if hasBodyFlag {
			method = "POST"
		} else {
			method = "GET"
		}
	}
	return method, urlValue, true
}

func flattenCommandTokens(command []string) []string {
	var tokens []string
	for _, arg := range command {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		tokens = append(tokens, arg)
		if strings.ContainsAny(arg, " \t\n") {
			tokens = append(tokens, strings.Fields(arg)...)
		}
	}
	return tokens
}

func normalizeCommandToken(token string) string {
	return strings.Trim(strings.TrimSpace(token), "\"'`")
}

func isHTTPMethod(token string) bool {
	switch strings.ToUpper(normalizeCommandToken(token)) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func normalizeContractPath(value string) (string, bool) {
	value = normalizeCommandToken(value)
	if value == "" {
		return "", false
	}
	if strings.HasPrefix(value, "/") {
		path, _, _ := strings.Cut(value, "?")
		if path == "" {
			path = "/"
		}
		return path, true
	}
	return extractRequestPath(value)
}

func extractRequestPath(value string) (string, bool) {
	parsed, err := url.Parse(normalizeCommandToken(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return path, true
}

func cliEvidenceIssues(store *Store, run Run, item Evidence) []string {
	var issues []string
	if item.Tier < TierRegisteredRun {
		issues = append(issues, fmt.Sprintf("cli evidence tier %d is below required tier %d", item.Tier, TierRegisteredRun))
	}
	if strings.TrimSpace(item.Provenance.SessionID) == "" {
		issues = append(issues, "cli evidence is missing a registered session id")
	}
	if item.CLI == nil {
		issues = append(issues, "cli evidence is missing parsed cli data")
	} else {
		if strings.TrimSpace(item.CLI.Command) == "" {
			issues = append(issues, "cli evidence is missing a command")
		}
		if strings.TrimSpace(item.CLI.Tool) == "" {
			issues = append(issues, "cli evidence is missing a tool name")
		}
	}
	issues = append(issues, assertionIssues(item.Assertions, "cli")...)

	hasScreenshot := false
	hasTranscript := false
	for _, artifact := range item.Artifacts {
		if err := store.VerifyArtifactHash(run, artifact); err != nil {
			issues = append(issues, fmt.Sprintf("artifact hash mismatch for %s", artifact.Label))
			continue
		}
		switch artifact.Kind {
		case ArtifactImage:
			hasScreenshot = true
		case ArtifactTranscript:
			hasTranscript = true
		}
	}
	if !hasScreenshot {
		issues = append(issues, "cli evidence is missing a screenshot")
	}
	if !hasTranscript {
		issues = append(issues, "cli evidence is missing a transcript")
	}
	return dedupeStrings(issues)
}

func assertionIssues(assertions []Assertion, surface string) []string {
	if len(assertions) == 0 {
		return []string{fmt.Sprintf("%s evidence has no assertions", surface)}
	}
	passCount := 0
	var issues []string
	for _, assertion := range assertions {
		if assertion.Result == AssertionPass {
			passCount++
			continue
		}
		if assertion.Result == AssertionFail {
			issue := fmt.Sprintf("assertion failed: %s", assertion.Description)
			if assertion.Expected != "" || assertion.Actual != "" {
				issue = fmt.Sprintf("%s (expected %s, actual %s)", issue, firstNonEmpty(assertion.Expected, "<empty>"), firstNonEmpty(assertion.Actual, "<empty>"))
			}
			if assertion.Message != "" {
				issue = fmt.Sprintf("%s: %s", issue, assertion.Message)
			}
			issues = append(issues, issue)
		}
	}
	if passCount == 0 {
		issues = append(issues, fmt.Sprintf("%s evidence has no passing assertions", surface))
	}
	return issues
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	var deduped []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	return deduped
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeTranscript(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
}

func transcriptPreview(value string) string {
	value = normalizeTranscript(value)
	value = strings.ReplaceAll(value, "\n", " / ")
	if len(value) <= 512 {
		return value
	}
	return value[:512]
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

func uiRequirementsForPlatform(platform string) (bool, bool, bool, bool) {
	switch normalizePlatform(platform) {
	case PlatformIOS:
		return false, true, false, false
	case PlatformCLI:
		return false, false, true, false
	case PlatformDesktop:
		return false, false, false, true
	default:
		return true, false, false, false
	}
}

func validateSurfaceForPlatform(surface, platform string) error {
	allowed := map[string][]string{
		PlatformWeb:     {SurfaceBrowser, SurfaceCurl},
		PlatformIOS:     {SurfaceIOS, SurfaceCurl},
		PlatformCLI:     {SurfaceCLI},
		PlatformDesktop: {SurfaceDesktop, SurfaceCurl},
	}
	for _, s := range allowed[normalizePlatform(platform)] {
		if s == surface {
			return nil
		}
	}
	return fmt.Errorf("%s evidence is not valid for a %s platform run", surface, platform)
}

// DefaultMaxScreenshotAge is the default maximum age for screenshot source files.
var DefaultMaxScreenshotAge = 30 * time.Minute

func detectDuplicateScreenshots(store *Store, run Run, currentScenarioID string, artifacts []Artifact) error {
	existing, err := store.LoadEvidence(run)
	if err != nil {
		return err
	}

	// Build index of image SHA256 → (scenario ID, artifact label) from other scenarios.
	type artifactRef struct {
		scenarioID string
		label      string
	}
	index := map[string]artifactRef{}
	for _, item := range existing {
		if item.ScenarioID == currentScenarioID {
			continue
		}
		for _, artifact := range item.Artifacts {
			if artifact.Kind != ArtifactImage {
				continue
			}
			if _, exists := index[artifact.SHA256]; !exists {
				index[artifact.SHA256] = artifactRef{scenarioID: item.ScenarioID, label: artifact.Label}
			}
		}
	}

	for _, artifact := range artifacts {
		if artifact.Kind != ArtifactImage {
			continue
		}
		if ref, exists := index[artifact.SHA256]; exists {
			return fmt.Errorf("screenshot %q has identical content to artifact %q in scenario %q; each scenario requires unique evidence", artifact.Label, ref.label, ref.scenarioID)
		}
	}
	return nil
}

func validateScreenshotFreshness(artifacts []Artifact, maxAge time.Duration) error {
	now := time.Now().UTC()
	for _, artifact := range artifacts {
		if artifact.Kind != ArtifactImage {
			continue
		}
		if artifact.SourceMtime.IsZero() {
			continue
		}
		age := now.Sub(artifact.SourceMtime)
		if age > maxAge {
			return fmt.Errorf("screenshot %q is too old (modified %s ago, max %s); capture a fresh screenshot", artifact.Label, age.Round(time.Second), maxAge)
		}
	}
	return nil
}

func checkAssertionFailures(assertions []Assertion) error {
	var failures []string
	for _, a := range assertions {
		if a.Result == AssertionFail {
			failures = append(failures, fmt.Sprintf("assertion failed: %s (expected %s, actual %s)", a.Description, a.Expected, a.Actual))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

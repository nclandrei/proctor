package proctor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func CreateRun(store *Store, cwd string, opts StartOptions) (Run, error) {
	repoRoot := RepoRoot(cwd)
	repoSlug, err := RepoSlug(repoRoot)
	if err != nil {
		return Run{}, err
	}
	now := time.Now().UTC()
	run := Run{
		ID:             newID("run"),
		RepoRoot:       repoRoot,
		RepoSlug:       repoSlug,
		Feature:        strings.TrimSpace(opts.Feature),
		BrowserURL:     strings.TrimSpace(opts.BrowserURL),
		CurlRequired:   strings.EqualFold(opts.CurlMode, "required"),
		CurlEndpoints:  normalizedLines(opts.CurlEndpoints),
		CurlSkipReason: strings.TrimSpace(opts.CurlSkipReason),
		Status:         StatusInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	run.HappyPath = Scenario{
		ID:              "happy-path",
		Label:           strings.TrimSpace(opts.HappyPath),
		Kind:            "happy-path",
		BrowserRequired: true,
		CurlRequired:    run.CurlRequired,
	}
	run.FailurePath = Scenario{
		ID:              "failure-path",
		Label:           strings.TrimSpace(opts.FailurePath),
		Kind:            "failure-path",
		BrowserRequired: true,
		CurlRequired:    run.CurlRequired,
	}
	run.Scenarios = append(run.Scenarios, run.HappyPath, run.FailurePath)

	edgeCategories, edgeScenarios := parseEdgeCaseInputs(opts.EdgeCaseInputs)
	run.EdgeCaseCategories = edgeCategories
	run.Scenarios = append(run.Scenarios, edgeScenarios...)

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
	return nil
}

func RecordCurl(store *Store, run Run, opts CurlRecordOptions) error {
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
	return nil
}

func Evaluate(store *Store, run Run) (Evaluation, error) {
	evidence, err := store.LoadEvidence(run)
	if err != nil {
		return Evaluation{}, err
	}

	eval := Evaluation{Complete: true}
	hasDesktop := false
	hasMobile := false

	for _, scenario := range run.Scenarios {
		scenarioEval := ScenarioEvaluation{Scenario: scenario}
		browserEvidence := selectEvidenceForScenario(evidence, scenario.ID, SurfaceBrowser)
		curlEvidence := selectEvidenceForScenario(evidence, scenario.ID, SurfaceCurl)

		if scenario.BrowserRequired {
			scenarioEval.BrowserOK, scenarioEval.BrowserIssues = validateBrowserEvidence(store, run, browserEvidence)
			if !scenarioEval.BrowserOK {
				eval.Complete = false
			} else {
				for _, item := range browserEvidence {
					for _, artifact := range item.Artifacts {
						if artifact.Kind != ArtifactImage {
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
			}
		}

		if scenario.CurlRequired {
			scenarioEval.CurlOK, scenarioEval.CurlIssues = validateCurlEvidence(store, run, curlEvidence)
			if !scenarioEval.CurlOK {
				eval.Complete = false
			}
		}

		eval.ScenarioEvaluations = append(eval.ScenarioEvaluations, scenarioEval)
	}

	if !hasDesktop {
		eval.Complete = false
		eval.GlobalMissing = append(eval.GlobalMissing, "at least one desktop screenshot")
	}
	if !hasMobile {
		eval.Complete = false
		eval.GlobalMissing = append(eval.GlobalMissing, "at least one mobile screenshot")
	}

	return eval, nil
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

func writeReports(store *Store, run Run) error {
	eval, err := Evaluate(store, run)
	if err != nil {
		return err
	}
	evidence, err := store.LoadEvidence(run)
	if err != nil {
		return err
	}
	markdown, html, err := RenderReports(run, eval, evidence)
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

func parseEdgeCaseInputs(inputs []string) ([]EdgeCaseCategory, []Scenario) {
	var categories []EdgeCaseCategory
	var scenarios []Scenario
	for _, category := range EdgeCaseCategories {
		entry := EdgeCaseCategory{Category: category}
		response := ""
		for _, input := range inputs {
			key, value, ok := strings.Cut(input, "=")
			if !ok {
				key, value, ok = strings.Cut(input, ":")
			}
			if !ok || !strings.EqualFold(strings.TrimSpace(key), category) {
				continue
			}
			response = strings.TrimSpace(value)
			break
		}
		if strings.EqualFold(response, "") {
			entry.Status = EdgeCategoryNA
			entry.Reason = "No answer recorded"
			categories = append(categories, entry)
			continue
		}
		if strings.HasPrefix(strings.ToLower(response), "n/a") {
			entry.Status = EdgeCategoryNA
			entry.Reason = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(response), "n/a"), ":"))
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
				BrowserRequired: true,
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

func validateBrowserEvidence(store *Store, run Run, items []Evidence) (bool, []string) {
	if len(items) == 0 {
		return false, []string{"missing browser evidence"}
	}
	for _, item := range items {
		issues := browserEvidenceIssues(store, run, item)
		if len(issues) == 0 {
			return true, nil
		}
	}
	var issues []string
	for _, item := range items {
		issues = append(issues, browserEvidenceIssues(store, run, item)...)
	}
	return false, dedupeStrings(issues)
}

func validateCurlEvidence(store *Store, run Run, items []Evidence) (bool, []string) {
	if len(items) == 0 {
		return false, []string{"missing curl evidence"}
	}
	for _, item := range items {
		issues := curlEvidenceIssues(store, run, item)
		if len(issues) == 0 {
			return true, nil
		}
	}
	var issues []string
	for _, item := range items {
		issues = append(issues, curlEvidenceIssues(store, run, item)...)
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

func browserEvidenceIssues(store *Store, run Run, item Evidence) []string {
	var issues []string
	if item.Tier < TierRegisteredRun {
		issues = append(issues, fmt.Sprintf("browser evidence tier %d is below required tier %d", item.Tier, TierRegisteredRun))
	}
	if item.Provenance.SessionID == "" {
		issues = append(issues, "browser evidence is missing a registered session id")
	}
	issues = append(issues, browserReportStructureIssues(item)...)
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

func curlEvidenceIssues(store *Store, run Run, item Evidence) []string {
	var issues []string
	if item.Tier < TierWrappedCommand {
		issues = append(issues, fmt.Sprintf("curl evidence tier %d is below required tier %d", item.Tier, TierWrappedCommand))
	}
	if len(item.Provenance.Command) == 0 {
		issues = append(issues, "curl evidence is missing a wrapped command")
	}
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

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

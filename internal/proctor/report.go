package proctor

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type scenarioReport struct {
	Eval     ScenarioEvaluation
	Evidence []Evidence
}

type scenarioHTMLReport struct {
	Eval       ScenarioEvaluation
	PreNotes   []PreNote
	LogEntries []logEntryHTMLReport
	Evidence   []evidenceHTMLReport
}

type logEntryHTMLReport struct {
	Step           int
	Action         string
	Observation    string
	Comparison     string
	InlineSource   template.URL
	ModalID        string
	ScreenshotPath string
	CreatedAt      time.Time
}

type evidenceHTMLReport struct {
	Surface    string
	Summary    []string
	Notes      string
	Assertions []Assertion
	Artifacts  []artifactHTMLReport
	CreatedAt  time.Time
}

type artifactHTMLReport struct {
	Label               string
	Path                string
	Kind                string
	InlineSource        template.URL
	InlineText          string
	HasInlineText       bool
	InlineTextLineCount int
	ModalID             string
}

func RenderReports(run Run, runDir string, eval Evaluation, evidence []Evidence, preNotes []PreNote, logEntries []ScreenshotLogEntry) (string, string, error) {
	scenarios := groupEvidenceByScenario(eval, evidence)
	htmlScenarios := buildScenarioHTMLReports(runDir, scenarios, preNotes, logEntries)
	edgeCoverage := edgeCoverageRows(run)
	curlRequirements := curlRequirementRows(run)
	runSurface := normalizePlatform(run.Platform)

	preNoteIndex := map[string][]PreNote{}
	for _, n := range preNotes {
		preNoteIndex[n.Scenario] = append(preNoteIndex[n.Scenario], n)
	}
	logIndex := map[string][]ScreenshotLogEntry{}
	for _, entry := range logEntries {
		logIndex[entry.ScenarioID] = append(logIndex[entry.ScenarioID], entry)
	}

	var md strings.Builder
	md.WriteString(fmt.Sprintf("# %s\n\n", run.Feature))
	md.WriteString(fmt.Sprintf("- Run ID: `%s`\n", run.ID))
	md.WriteString(fmt.Sprintf("- Platform: `%s`\n", normalizePlatform(run.Platform)))
	md.WriteString(fmt.Sprintf("- Verification surface: `%s`\n", surfaceTitle(runSurface)))
	switch runSurface {
	case PlatformCLI:
		md.WriteString(fmt.Sprintf("- CLI command: `%s`\n", run.CLICommand))
	case PlatformIOS:
		md.WriteString(fmt.Sprintf("- iOS scheme: `%s`\n", run.IOS.Scheme))
		md.WriteString(fmt.Sprintf("- iOS bundle ID: `%s`\n", run.IOS.BundleID))
		if strings.TrimSpace(run.IOS.Simulator) != "" {
			md.WriteString(fmt.Sprintf("- Simulator: `%s`\n", run.IOS.Simulator))
		}
	case PlatformDesktop:
		md.WriteString(fmt.Sprintf("- Desktop app: `%s`\n", run.Desktop.Name))
		if strings.TrimSpace(run.Desktop.BundleID) != "" {
			md.WriteString(fmt.Sprintf("- Bundle ID: `%s`\n", run.Desktop.BundleID))
		}
	default:
		md.WriteString(fmt.Sprintf("- Browser URL: `%s`\n", run.BrowserURL))
	}
	if runHasHTTPSummary(run) {
		if len(curlRequirements) > 0 {
			md.WriteString(fmt.Sprintf("- Direct HTTP verification: required for %d scenario(s)\n", len(curlRequirements)))
		} else if strings.TrimSpace(run.CurlSkipReason) != "" {
			md.WriteString(fmt.Sprintf("- Direct HTTP verification: skipped (%s)\n", run.CurlSkipReason))
		} else {
			md.WriteString("- Direct HTTP verification: skipped\n")
		}
	}
	if len(curlRequirements) > 0 {
		md.WriteString("- HTTP risk coverage:\n")
		for _, item := range curlRequirements {
			md.WriteString(fmt.Sprintf("  - %s (`%s`)\n", item.Label, item.ID))
			for _, endpoint := range item.Endpoints {
				md.WriteString(fmt.Sprintf("    - `%s`\n", endpoint))
			}
		}
	}
	if len(edgeCoverage) > 0 {
		md.WriteString("\n## Edge Case Coverage\n\n")
		for _, row := range edgeCoverage {
			md.WriteString(fmt.Sprintf("### %s\n\n", row.Category))
			if row.Status == EdgeCategoryNA {
				md.WriteString(fmt.Sprintf("- Status: N/A (%s)\n\n", row.Reason))
				continue
			}
			md.WriteString("- Status: scenario coverage required\n")
			for _, label := range row.ScenarioLabels {
				md.WriteString(fmt.Sprintf("  - %s\n", label))
			}
			md.WriteString("\n")
		}
	}
	md.WriteString("\n## Scenario Status\n\n")
	for _, scenario := range scenarios {
		// Scenario status + contract claim
		md.WriteString(fmt.Sprintf("### %s\n\n", scenario.Eval.Scenario.Label))
		for _, surface := range scenario.Eval.Scenario.RequiredSurfaces() {
			ok, _ := scenario.Eval.SurfaceStatus(surface)
			if ok {
				md.WriteString(fmt.Sprintf("- %s: pass\n", strings.ToUpper(surface)))
				continue
			}
			md.WriteString(fmt.Sprintf("- %s: fail (%s)\n", strings.ToUpper(surface), strings.Join(scenario.Eval.SurfaceIssues(surface), ", ")))
		}

		// Pre-test notes
		preNotes := preNoteIndex[scenario.Eval.Scenario.ID]
		if len(preNotes) > 0 {
			md.WriteString("\n**Pre-test notes:**\n\n")
			for _, n := range preNotes {
				md.WriteString(fmt.Sprintf("> %s\n", n.Notes))
				md.WriteString(fmt.Sprintf("> — %s", formatTimestamp(n.CreatedAt)))
				if n.Session != "" {
					md.WriteString(fmt.Sprintf(" · session: `%s`", n.Session))
				}
				md.WriteString("\n\n")
			}
		}

		// Verification (renamed from observation notes)
		for _, item := range scenario.Evidence {
			if item.Notes != "" {
				md.WriteString(fmt.Sprintf("\n> **Verification:** %s\n\n", item.Notes))
			}
		}

		// Assertions
		for _, item := range scenario.Evidence {
			for _, assertion := range item.Assertions {
				icon := "FAIL"
				if assertion.Result == AssertionPass {
					icon = "PASS"
				}
				md.WriteString(fmt.Sprintf("- [%s] `%s`\n", icon, assertion.Description))
				if assertion.Expected != "" || assertion.Actual != "" {
					md.WriteString(fmt.Sprintf("  - expected: `%s`\n", assertion.Expected))
					md.WriteString(fmt.Sprintf("  - actual: `%s`\n", assertion.Actual))
				}
				if assertion.Message != "" {
					md.WriteString(fmt.Sprintf("  - note: %s\n", assertion.Message))
				}
			}
		}

		// Artifact links
		for _, item := range scenario.Evidence {
			for _, artifact := range item.Artifacts {
				md.WriteString(fmt.Sprintf("- Artifact: [%s](%s)\n", artifact.Label, artifact.Path))
				if artifact.Kind == ArtifactImage {
					md.WriteString(fmt.Sprintf("  \n  ![%s](%s)\n", artifact.Label, artifact.Path))
				}
			}
		}

		// Technical metadata
		md.WriteString(fmt.Sprintf("\n- Scenario ID: `%s`\n", scenario.Eval.Scenario.ID))
		if scenario.Eval.Scenario.CurlRequired && len(scenario.Eval.Scenario.CurlEndpoints) > 0 {
			md.WriteString(fmt.Sprintf("- curl contract: `%s`\n", strings.Join(scenario.Eval.Scenario.CurlEndpoints, "`, `")))
		}
		for _, item := range scenario.Evidence {
			md.WriteString(fmt.Sprintf("\n<details><summary>%s evidence details</summary>\n\n", titleCase(item.Surface)))
			for _, line := range evidenceSummaryLines(item) {
				md.WriteString(fmt.Sprintf("- %s\n", line))
			}
			md.WriteString("\n</details>\n")
		}

		// Screenshot log (step-by-step verification walkthrough)
		scenarioLogs := logIndex[scenario.Eval.Scenario.ID]
		if len(scenarioLogs) > 0 {
			md.WriteString("\n<details><summary>Verification steps</summary>\n\n")
			for _, entry := range scenarioLogs {
				md.WriteString(fmt.Sprintf("**Step %d** (%s)\n\n", entry.Step, formatTimestamp(entry.CreatedAt)))
				md.WriteString(fmt.Sprintf("- **Action:** %s\n", entry.Action))
				md.WriteString(fmt.Sprintf("- **Observation:** %s\n", entry.Observation))
				md.WriteString(fmt.Sprintf("- **Comparison:** %s\n", entry.Comparison))
				if entry.ScreenshotPath != "" {
					md.WriteString(fmt.Sprintf("- Screenshot: ![step %d](%s)\n", entry.Step, entry.ScreenshotPath))
				}
				md.WriteString("\n")
			}
			md.WriteString("</details>\n")
		}

		md.WriteString("\n")
	}

	if len(eval.GlobalMissing) > 0 {
		md.WriteString("## Global Gaps\n\n")
		for _, item := range eval.GlobalMissing {
			md.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}

	type reportData struct {
		Run          Run
		Eval         Evaluation
		Scenarios    []scenarioHTMLReport
		EdgeCoverage []edgeCoverageRow
		CurlCoverage []curlRequirementRow
	}
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"join":                 strings.Join,
		"title":                titleCase,
		"evidenceSummaryLines": evidenceSummaryLines,
		"reportStatus":         reportStatus,
		"scenarioComplete":     scenarioComplete,
		"scenarioTone":         scenarioTone,
		"surfaceIssues":        surfaceIssues,
		"surfaceOK":            surfaceOK,
		"surfaceTitle":         surfaceTitle,
		"scenarioSurfaces":     scenarioSurfaces,
		"passedScenarioCount":  passedScenarioCount,
		"failedScenarioCount":  failedScenarioCount,
		"totalScenarioCount":   totalScenarioCount,
		"curlModeLabel":        curlModeLabel,
		"runHasHTTPSummary":    runHasHTTPSummary,
		"runSurfaceLabel":      runSurfaceLabel,
		"runTargetLabel":       runTargetLabel,
		"runTargetValue":       runTargetValue,
		"formatTimestamp":      formatTimestamp,
		"truncateActual":       truncateActual,
		"allAssertionsPass":    allAssertionsPass,
	}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light">
  <title>{{ .Run.Feature }} · Proctor</title>
  <style>
    :root { --bg: #ffffff; --border: #d8dee4; --muted: #59636e; --pass: #1a7f37; --pass-bg: #dafbe1; --fail: #cf222e; --fail-bg: #ffebe9; }
    * { box-sizing: border-box; }
    body { max-width: 760px; margin: 32px auto; padding: 0 20px; font: 14px/1.6 -apple-system,"Segoe UI",Helvetica,Arial,sans-serif; color: #1f2328; background: var(--bg); color-scheme: light; }
    h1 { font-size: 1.5em; margin: 0 0 2px; }
    h2 { font-size: 1.15em; margin: 28px 0 6px; padding-bottom: 4px; border-bottom: 1px solid var(--border); }
    h3 { font-size: 1em; margin: 12px 0 4px; }
    h4 { font-size: 0.8em; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); margin: 10px 0 4px; }
    code { font: 0.9em/1 ui-monospace,SFMono-Regular,Menlo,monospace; background: #f0f0f0; padding: 1px 4px; border-radius: 3px; }
    a { color: #0969da; }
    figure { margin: 0; }
    ul, ol { padding-left: 20px; }
    .pass { color: var(--pass); }
    .fail { color: var(--fail); }
    .badge { display: inline-block; font: 600 0.75em ui-monospace,monospace; padding: 1px 6px; border-radius: 3px; }
    .badge.ok, .badge.pass { background: var(--pass-bg); color: var(--pass); }
    .badge.bad, .badge.fail { background: var(--fail-bg); color: var(--fail); }
    .muted { color: var(--muted); font-size: 0.85em; }
    .small { font-size: 0.8em; }
    .detail { display: block; font-size: 0.85em; color: var(--muted); margin-left: 2px; }
    .summary-line { margin: 12px 0; font-size: 0.95em; }
    .meta-list { padding-left: 20px; font-size: 0.9em; margin: 8px 0; }
    .meta-list li { margin: 2px 0; }
    .edge-list { padding-left: 20px; margin: 8px 0; }
    .edge-list li { margin: 4px 0; font-size: 0.9em; }
    .edge-list .edge-scenarios { padding-left: 20px; font-size: 0.85em; color: var(--muted); margin: 2px 0; }
    details.scenario { border: 1px solid var(--border); border-radius: 4px; margin: 6px 0; }
    details.scenario > summary { padding: 8px 12px; cursor: pointer; font-size: 0.9em; list-style: none; }
    details.scenario > summary::-webkit-details-marker { display: none; }
    details.scenario > summary::marker { content: ""; }
    .scenario-body { padding: 6px 12px 12px; border-top: 1px solid var(--border); }
    .scenario-id { margin: 6px 0; }
    .evidence { margin: 10px 0; padding-top: 8px; border-top: 1px solid var(--border); }
    .evidence-timestamp { font-size: 0.78em; color: var(--muted); margin-bottom: 4px; }
    .kv-list { list-style: none; padding: 0; margin: 4px 0; }
    .kv-list li { font-size: 0.85em; color: var(--muted); }
    .assertion-list { list-style: none; padding: 0; }
    .assertion-list li { padding: 3px 0; font-size: 0.88em; }
    .assertion-tag { font: 600 0.7em ui-monospace,monospace; padding: 1px 4px; border-radius: 2px; margin-right: 4px; }
    .assertion-tag.pass { background: var(--pass-bg); color: var(--pass); }
    .assertion-tag.fail { background: var(--fail-bg); color: var(--fail); }
    .issue-list { padding-left: 20px; font-size: 0.85em; color: var(--fail); }
    .artifact-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 8px; margin: 8px 0; }
    .artifact { border: 1px solid var(--border); border-radius: 3px; overflow: hidden; }
    .artifact.artifact-wide { grid-column: 1 / -1; }
    .artifact-head { display: flex; justify-content: space-between; padding: 4px 8px; font-size: 0.8em; }
    .artifact-name { font-weight: 600; }
    .artifact-file { color: var(--muted); font-size: 0.85em; }
    .thumb { display: block; cursor: zoom-in; }
    .thumb img { width: 100%; max-height: 180px; object-fit: contain; display: block; border-top: 1px solid var(--border); }
    .thumb-note { font-size: 0.7em; color: var(--muted); padding: 2px 8px; }
    .screenshot-gallery { margin: 12px 0; }
    .screenshot-gallery .screenshot-item { margin: 8px 0; border: 1px solid var(--border); border-radius: 4px; overflow: hidden; }
    .screenshot-gallery .screenshot-item .artifact-head { padding: 4px 8px; font-size: 0.8em; background: #f6f8fa; }
    .screenshot-gallery .screenshot-item a.thumb img { width: 100%; min-width: 400px; max-height: 400px; object-fit: contain; display: block; border-top: 1px solid var(--border); }
    .verification-box { margin: 12px 0; padding: 8px 12px; background: #dafbe1; border-left: 3px solid var(--pass); border-radius: 2px; }
    .verification-box .verification-contract { font-size: 0.78em; color: var(--muted); margin-bottom: 4px; }
    .verification-box .verification-label { font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.04em; color: var(--pass); font-weight: 600; margin-bottom: 2px; }
    .verification-box .verification-text { font-size: 0.85em; white-space: pre-wrap; }
    .evidence-details { margin: 8px 0; }
    .evidence-details > summary { font-size: 0.82em; color: var(--muted); cursor: pointer; padding: 4px 0; }
    .evidence-details .evidence-meta { padding: 6px 0; }
    .transcript { border-top: 1px solid var(--border); }
    .transcript > summary { padding: 6px 8px; cursor: pointer; font-size: 0.82em; color: var(--muted); }
    .line-count { font-size: 0.8em; color: #0969da; }
    .log { margin: 0; padding: 8px; font: 0.8em/1.5 ui-monospace,monospace; max-height: 300px; overflow: auto; border-top: 1px solid var(--border); white-space: pre-wrap; word-break: break-word; }
    .lightbox { position: fixed; inset: 0; display: none; z-index: 100; padding: 20px; background: rgba(0,0,0,0.65); }
    .lightbox:target { display: grid; place-items: center; }
    .lightbox-bg { position: absolute; inset: 0; }
    .lightbox-panel { position: relative; z-index: 1; max-width: 900px; width: 100%; max-height: calc(100vh - 40px); overflow: auto; background: #fff; border-radius: 4px; padding: 12px; }
    .lightbox-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; font-weight: 600; }
    .lightbox-close { font-size: 0.85em; padding: 2px 8px; border: 1px solid var(--border); border-radius: 3px; color: var(--muted); text-decoration: none; }
    .lightbox-panel img { width: 100%; height: auto; display: block; }
    .global-gaps { margin: 28px 0; padding: 10px 14px; border-left: 3px solid var(--fail); background: var(--fail-bg); }
    .global-gaps h2 { border: none; margin: 0 0 6px; padding: 0; }
    .global-gaps ul { padding-left: 20px; font-size: 0.88em; }
    .notes { margin: 8px 0; padding: 6px 10px; background: #f6f8fa; border-left: 3px solid #d0d7de; border-radius: 2px; font-size: 0.85em; white-space: pre-wrap; }
    .notes-label { font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); font-weight: 600; margin-bottom: 2px; }
    .pre-notes { margin: 8px 0; }
    .pre-note { margin: 4px 0; padding: 6px 10px; background: #fff8c5; border-left: 3px solid #d4a72c; border-radius: 2px; font-size: 0.85em; }
    .pre-note-meta { font-size: 0.78em; color: var(--muted); margin-bottom: 2px; }
    @media print { .scenario { break-inside: avoid; } .lightbox { display: none !important; } }
  </style>
</head>
<body>
  <p style="font-size:0.7em;text-transform:uppercase;letter-spacing:0.06em;color:var(--muted);margin-bottom:2px;">Verification Report</p>
  <h1>{{ .Run.Feature }}</h1>

  <p class="summary-line">{{ totalScenarioCount .Scenarios }} scenarios: <span class="pass">{{ passedScenarioCount .Scenarios }} passed</span>{{ if failedScenarioCount .Scenarios }}, <span class="fail">{{ failedScenarioCount .Scenarios }} failed</span>{{ end }} — <span class="badge {{ if .Eval.Complete }}ok{{ else }}bad{{ end }}">{{ reportStatus .Eval }}</span></p>

  <ul class="meta-list">
    <li>Run ID: <code>{{ .Run.ID }}</code></li>
    <li>Surface: {{ runSurfaceLabel .Run }}</li>
    <li>{{ runTargetLabel .Run }}: <code>{{ runTargetValue .Run }}</code></li>
    {{ if runHasHTTPSummary .Run }}<li>HTTP verification: {{ curlModeLabel .Run }}</li>{{ end }}
  </ul>

  {{ if .CurlCoverage }}
  <h3>HTTP Risk Coverage</h3>
  <ul class="meta-list">
    {{ range .CurlCoverage }}<li><strong>{{ .Label }}</strong> <code>{{ .ID }}</code>{{ if .Endpoints }} — {{ range $i, $ep := .Endpoints }}{{ if $i }}, {{ end }}<code>{{ $ep }}</code>{{ end }}{{ end }}</li>{{ end }}
  </ul>
  {{ end }}

  {{ if .EdgeCoverage }}
  <h2>Edge Case Coverage</h2>
  <ul class="edge-list">
    {{ range .EdgeCoverage }}
    <li><strong>{{ .Category }}</strong> — {{ if eq .Status "na" }}N/A ({{ .Reason }}){{ else }}Scenario coverage required<ul class="edge-scenarios">{{ range .ScenarioLabels }}<li>{{ . }}</li>{{ end }}</ul>{{ end }}</li>
    {{ end }}
  </ul>
  {{ end }}

  <h2>Scenarios</h2>
  {{ range $i, $s := .Scenarios }}
  {{ $eval := $s.Eval }}
  <details class="scenario" id="scenario-{{ $i }}"{{ if not (scenarioComplete $eval) }} open{{ end }}>
    <summary><span class="badge {{ scenarioTone $eval }}">{{ if scenarioComplete $eval }}PASS{{ else }}FAIL{{ end }}</span> {{ $eval.Scenario.Label }} <span class="muted">({{ title $eval.Scenario.Kind }})</span> — {{ range scenarioSurfaces $eval.Scenario }}{{ if surfaceOK $eval . }}<span class="pass">{{ surfaceTitle . }} ✓</span> {{ else }}<span class="fail">{{ surfaceTitle . }} ✗</span> {{ end }}{{ end }}</summary>
    <div class="scenario-body">
      {{/* 1. Scenario contract claim */}}
      <p style="font-size:1.05em;margin:4px 0 10px;"><strong>{{ $eval.Scenario.Label }}</strong></p>
      {{ range scenarioSurfaces $eval.Scenario }}{{ if not (surfaceOK $eval .) }}<ul class="issue-list">{{ range surfaceIssues $eval . }}<li>{{ . }}</li>{{ end }}</ul>{{ end }}{{ end }}

      {{/* 2. Pre-test notes */}}
      {{ if $s.PreNotes }}
      <div class="pre-notes">
        <div class="notes-label">Pre-test notes</div>
        {{ range $s.PreNotes }}
        <div class="pre-note">
          <div class="pre-note-meta">{{ formatTimestamp .CreatedAt }}{{ if .Session }} · session: <code>{{ .Session }}</code>{{ end }}</div>
          {{ .Notes }}
        </div>
        {{ end }}
      </div>
      {{ end }}

      {{/* 3. Screenshots — shown large and prominent */}}
      {{ if $s.LogEntries }}
      <div class="screenshot-gallery">
        <div class="notes-label">Verification Steps</div>
        {{ range $s.LogEntries }}
        {{ if .InlineSource }}
        <div class="screenshot-item">
          <div class="artifact-head"><span class="artifact-name">Step {{ .Step }}</span><span class="muted">{{ .Action }}</span></div>
          <a class="thumb" href="#{{ .ModalID }}"><img src="{{ .InlineSource }}" alt="Step {{ .Step }}"></a>
          <div class="lightbox" id="{{ .ModalID }}">
            <a class="lightbox-bg" href="#" aria-label="Close"></a>
            <figure class="lightbox-panel">
              <div class="lightbox-head"><figcaption>Step {{ .Step }}</figcaption><a class="lightbox-close" href="#">Close</a></div>
              <img src="{{ .InlineSource }}" alt="Step {{ .Step }}">
            </figure>
          </div>
        </div>
        {{ end }}
        {{ end }}
      </div>
      {{ end }}
      {{ if not $s.LogEntries }}
      {{ range $s.Evidence }}
      {{ if .Artifacts }}
      <div class="screenshot-gallery">
        {{ range .Artifacts }}
        {{ if .InlineSource }}
        <div class="screenshot-item">
          <div class="artifact-head"><span class="artifact-name">{{ .Label }}</span><a class="artifact-file" href="{{ .Path }}">file</a></div>
          <a class="thumb" href="#{{ .ModalID }}"><img src="{{ .InlineSource }}" alt="{{ .Label }}"></a>
          <div class="thumb-note">Click to enlarge</div>
          <div class="lightbox" id="{{ .ModalID }}">
            <a class="lightbox-bg" href="#" aria-label="Close"></a>
            <figure class="lightbox-panel">
              <div class="lightbox-head"><figcaption>{{ .Label }}</figcaption><a class="lightbox-close" href="#">Close</a></div>
              <img src="{{ .InlineSource }}" alt="{{ .Label }}">
            </figure>
          </div>
        </div>
        {{ end }}
        {{ end }}
      </div>
      {{ end }}
      {{ end }}
      {{ end }}

      {{/* 4. Verification — renamed from observation notes, with contract claim */}}
      {{ range $s.Evidence }}
      {{ if .Notes }}
      <div class="verification-box">
        <div class="verification-contract">Contract: {{ $eval.Scenario.Label }}</div>
        <div class="verification-label">Verification</div>
        <div class="verification-text">{{ .Notes }}</div>
      </div>
      {{ end }}
      {{ end }}

      {{/* 5. Failed assertions shown inline; all-pass and technical details collapsed together */}}
      {{ range $s.Evidence }}
      {{ if not (allAssertionsPass .Assertions) }}
      <ul class="assertion-list">
        {{ range .Assertions }}{{ if ne .Result "pass" }}
        <li>
          <span class="assertion-tag fail">FAIL</span>
          <code>{{ .Description }}</code>
          {{ if or .Expected .Actual }}<span class="detail">expected: <code>{{ .Expected }}</code> · actual: <code>{{ .Actual }}</code></span>{{ end }}
          {{ if .Message }}<span class="detail">{{ .Message }}</span>{{ end }}
        </li>
        {{ end }}{{ end }}
      </ul>
      {{ end }}
      {{ end }}

      <details class="evidence-details">
        <summary>Technical details{{ range $s.Evidence }}{{ if .Assertions }} · <span class="badge {{ if allAssertionsPass .Assertions }}ok{{ else }}bad{{ end }}">{{ len .Assertions }} assertions</span>{{ end }}{{ if not .CreatedAt.IsZero }} · <span class="evidence-timestamp">Captured: {{ formatTimestamp .CreatedAt }}</span>{{ end }}{{ end }}</summary>
        {{ range $s.Evidence }}
        <div class="evidence-meta">
          {{ if .Assertions }}
          <ul class="assertion-list">
            {{ range .Assertions }}
            <li>
              <span class="assertion-tag {{ if eq .Result "pass" }}pass{{ else }}fail{{ end }}">{{ if eq .Result "pass" }}PASS{{ else }}FAIL{{ end }}</span>
              <code>{{ .Description }}</code>
            </li>
            {{ end }}
          </ul>
          {{ end }}
          {{ if .Summary }}<ul class="kv-list">{{ range .Summary }}<li>{{ . }}</li>{{ end }}</ul>{{ end }}
          {{ if .Artifacts }}
          {{ range .Artifacts }}
          {{ if .HasInlineText }}
          <div class="artifact artifact-wide">
            <div class="artifact-head"><span class="artifact-name">{{ .Label }}</span><a class="artifact-file" href="{{ .Path }}">file</a></div>
            <details class="transcript">
              <summary><span>Transcript</span> <span class="line-count">{{ if gt .InlineTextLineCount 0 }}{{ .InlineTextLineCount }} lines{{ else }}empty{{ end }}</span></summary>
              <pre class="log">{{ .InlineText }}</pre>
            </details>
          </div>
          {{ else if not .InlineSource }}
          <div class="artifact">
            <div class="artifact-head"><span class="artifact-name">{{ .Label }}</span><a class="artifact-file" href="{{ .Path }}">file</a></div>
            <p class="muted small" style="padding:4px 8px;">Linked artifact</p>
          </div>
          {{ end }}
          {{ end }}
          {{ end }}
        </div>
        {{ end }}
        {{ if $s.LogEntries }}
        {{ range $s.LogEntries }}
        <div style="padding:4px 0;border-bottom:1px solid var(--border);">
          <h4>Step {{ .Step }}</h4>
          {{ if not .CreatedAt.IsZero }}<div class="evidence-timestamp">{{ formatTimestamp .CreatedAt }}</div>{{ end }}
          <ul class="kv-list">
            <li><strong>Action:</strong> {{ .Action }}</li>
            <li><strong>Observation:</strong> {{ .Observation }}</li>
            <li><strong>Comparison:</strong> {{ .Comparison }}</li>
          </ul>
        </div>
        {{ end }}
        {{ end }}
      </details>

      <div class="scenario-id muted" style="margin-top:8px;"><code>{{ $eval.Scenario.ID }}</code>{{ if and $eval.Scenario.CurlRequired $eval.Scenario.CurlEndpoints }} · curl: {{ range $ci, $ep := $eval.Scenario.CurlEndpoints }}{{ if $ci }}, {{ end }}<code>{{ $ep }}</code>{{ end }}{{ end }}</div>
    </div>
  </details>
  {{ end }}

  {{ if .Eval.GlobalMissing }}
  <div class="global-gaps">
    <h2>Global Gaps</h2>
    <ul>{{ range .Eval.GlobalMissing }}<li>{{ . }}</li>{{ end }}</ul>
  </div>
  {{ end }}
</body>
</html>`)
	if err != nil {
		return "", "", err
	}

	var html strings.Builder
	if err := tmpl.Execute(&html, reportData{
		Run:          run,
		Eval:         eval,
		Scenarios:    htmlScenarios,
		EdgeCoverage: edgeCoverage,
		CurlCoverage: curlRequirements,
	}); err != nil {
		return "", "", err
	}
	return md.String(), html.String(), nil
}

func groupEvidenceByScenario(eval Evaluation, evidence []Evidence) []scenarioReport {
	index := map[string][]Evidence{}
	for _, item := range evidence {
		index[item.ScenarioID] = append(index[item.ScenarioID], item)
	}

	var reports []scenarioReport
	for _, item := range eval.ScenarioEvaluations {
		report := scenarioReport{
			Eval:     item,
			Evidence: append([]Evidence(nil), index[item.Scenario.ID]...),
		}
		sort.Slice(report.Evidence, func(i, j int) bool {
			if report.Evidence[i].Surface == report.Evidence[j].Surface {
				return report.Evidence[i].CreatedAt.Before(report.Evidence[j].CreatedAt)
			}
			return report.Evidence[i].Surface < report.Evidence[j].Surface
		})
		reports = append(reports, report)
	}
	return reports
}

type edgeCoverageRow struct {
	Category       string
	Status         string
	Reason         string
	ScenarioLabels []string
}

type curlRequirementRow struct {
	Label     string
	ID        string
	Endpoints []string
}

func edgeCoverageRows(run Run) []edgeCoverageRow {
	scenarioLabels := map[string]string{}
	for _, scenario := range run.Scenarios {
		scenarioLabels[scenario.ID] = scenario.Label
	}

	var rows []edgeCoverageRow
	for _, category := range run.EdgeCaseCategories {
		row := edgeCoverageRow{
			Category: category.Category,
			Status:   category.Status,
			Reason:   category.Reason,
		}
		for _, scenarioID := range category.ScenarioIDs {
			if label := scenarioLabels[scenarioID]; label != "" {
				row.ScenarioLabels = append(row.ScenarioLabels, label)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func curlRequirementRows(run Run) []curlRequirementRow {
	var rows []curlRequirementRow
	for _, scenario := range run.Scenarios {
		if !scenario.CurlRequired {
			continue
		}
		row := curlRequirementRow{
			Label:     scenario.Label,
			ID:        scenario.ID,
			Endpoints: append([]string(nil), scenario.CurlEndpoints...),
		}
		rows = append(rows, row)
	}
	return rows
}

func buildScenarioHTMLReports(runDir string, scenarios []scenarioReport, preNotes []PreNote, logEntries []ScreenshotLogEntry) []scenarioHTMLReport {
	preNoteIndex := map[string][]PreNote{}
	for _, n := range preNotes {
		preNoteIndex[n.Scenario] = append(preNoteIndex[n.Scenario], n)
	}
	logIndex := map[string][]ScreenshotLogEntry{}
	for _, entry := range logEntries {
		logIndex[entry.ScenarioID] = append(logIndex[entry.ScenarioID], entry)
	}
	reports := make([]scenarioHTMLReport, 0, len(scenarios))
	for scenarioIdx, scenario := range scenarios {
		htmlScenario := scenarioHTMLReport{
			Eval:     scenario.Eval,
			PreNotes: preNoteIndex[scenario.Eval.Scenario.ID],
		}
		for logIdx, entry := range logIndex[scenario.Eval.Scenario.ID] {
			art := Artifact{Kind: ArtifactImage, Path: entry.ScreenshotPath}
			htmlScenario.LogEntries = append(htmlScenario.LogEntries, logEntryHTMLReport{
				Step:           entry.Step,
				Action:         entry.Action,
				Observation:    entry.Observation,
				Comparison:     entry.Comparison,
				InlineSource:   inlineArtifactDataURI(runDir, art),
				ModalID:        fmt.Sprintf("log-%d-%d", scenarioIdx, logIdx),
				ScreenshotPath: entry.ScreenshotPath,
				CreatedAt:      entry.CreatedAt,
			})
		}
		for evidenceIdx, item := range scenario.Evidence {
			htmlEvidence := evidenceHTMLReport{
				Surface:    item.Surface,
				Summary:    evidenceSummaryLines(item),
				Notes:      item.Notes,
				Assertions: append([]Assertion(nil), item.Assertions...),
				CreatedAt:  item.CreatedAt,
			}
			for artifactIdx, artifact := range item.Artifacts {
				inlineText, inlineTextLineCount, hasInlineText := inlineArtifactText(runDir, artifact)
				htmlEvidence.Artifacts = append(htmlEvidence.Artifacts, artifactHTMLReport{
					Label:               artifact.Label,
					Path:                artifact.Path,
					Kind:                artifact.Kind,
					InlineSource:        inlineArtifactDataURI(runDir, artifact),
					InlineText:          inlineText,
					HasInlineText:       hasInlineText,
					InlineTextLineCount: inlineTextLineCount,
					ModalID:             fmt.Sprintf("artifact-preview-%d-%d-%d", scenarioIdx, evidenceIdx, artifactIdx),
				})
			}
			htmlScenario.Evidence = append(htmlScenario.Evidence, htmlEvidence)
		}
		reports = append(reports, htmlScenario)
	}
	return reports
}

func inlineArtifactDataURI(runDir string, artifact Artifact) template.URL {
	if artifact.Kind != ArtifactImage {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(runDir, artifact.Path))
	if err != nil {
		return ""
	}
	return template.URL("data:" + artifactMediaType(artifact) + ";base64," + base64.StdEncoding.EncodeToString(data))
}

func inlineArtifactText(runDir string, artifact Artifact) (string, int, bool) {
	if artifact.Kind != ArtifactTranscript {
		return "", 0, false
	}
	data, err := os.ReadFile(filepath.Join(runDir, artifact.Path))
	if err != nil {
		return "", 0, false
	}
	text := string(data)
	return text, textLineCount(text), true
}

func artifactMediaType(artifact Artifact) string {
	if strings.TrimSpace(artifact.MediaType) != "" {
		return artifact.MediaType
	}
	if mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(artifact.Path))); mediaType != "" {
		return mediaType
	}
	return "application/octet-stream"
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func textLineCount(value string) int {
	if value == "" {
		return 0
	}
	count := strings.Count(value, "\n")
	if strings.HasSuffix(value, "\n") {
		return count
	}
	return count + 1
}

func evidenceSummaryLines(item Evidence) []string {
	switch item.Surface {
	case SurfaceBrowser:
		return browserSummaryLines(item)
	case SurfaceIOS:
		return iosSummaryLines(item)
	case SurfaceCurl:
		return curlSummaryLines(item)
	case SurfaceCLI:
		return cliSummaryLines(item)
	case SurfaceDesktop:
		return desktopSummaryLines(item)
	default:
		return nil
	}
}

func browserSummaryLines(item Evidence) []string {
	if item.Browser == nil {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Tool: `%s`", item.Browser.Tool),
		fmt.Sprintf("Session: `%s`", item.Browser.SessionID),
		fmt.Sprintf("Desktop final URL: `%s`", item.Browser.Desktop.FinalURL),
		fmt.Sprintf("Desktop issues: console=%d, page=%d, failed_requests=%d, http=%d", item.Browser.Desktop.ConsoleErrors, item.Browser.Desktop.PageErrors, item.Browser.Desktop.FailedRequests, item.Browser.Desktop.HTTPErrors),
	}
	if item.Browser.Mobile != nil {
		lines = append(lines,
			fmt.Sprintf("Mobile final URL: `%s`", item.Browser.Mobile.FinalURL),
			fmt.Sprintf("Mobile issues: console=%d, page=%d, failed_requests=%d, http=%d", item.Browser.Mobile.ConsoleErrors, item.Browser.Mobile.PageErrors, item.Browser.Mobile.FailedRequests, item.Browser.Mobile.HTTPErrors),
		)
	}
	return lines
}

func iosSummaryLines(item Evidence) []string {
	if item.IOS == nil {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Tool: `%s`", item.Provenance.Tool),
		fmt.Sprintf("Session: `%s`", item.Provenance.SessionID),
		fmt.Sprintf("Bundle ID: `%s`", item.IOS.BundleID),
		fmt.Sprintf("Screen: `%s`", item.IOS.Screen),
		fmt.Sprintf("Simulator: `%s`", item.IOS.Simulator.Name),
	}
	if strings.TrimSpace(item.IOS.Simulator.Runtime) != "" {
		lines = append(lines, fmt.Sprintf("Runtime: `%s`", item.IOS.Simulator.Runtime))
	}
	if strings.TrimSpace(item.IOS.State) != "" {
		lines = append(lines, fmt.Sprintf("State: `%s`", item.IOS.State))
	}
	lines = append(lines, fmt.Sprintf("Issues: launch_errors=%d, crashes=%d, fatal_logs=%d", item.IOS.Issues.LaunchErrors, item.IOS.Issues.Crashes, item.IOS.Issues.FatalLogs))
	return lines
}

func curlSummaryLines(item Evidence) []string {
	if item.Curl == nil {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Command: `%s`", strings.Join(item.Curl.Command, " ")),
		fmt.Sprintf("Exit code: `%d`", item.Curl.ExitCode),
	}
	if item.Curl.ResponseStatus != 0 {
		lines = append(lines, fmt.Sprintf("Response status: `%d`", item.Curl.ResponseStatus))
	}
	return lines
}

func cliSummaryLines(item Evidence) []string {
	if item.CLI == nil {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Tool: `%s`", item.CLI.Tool),
		fmt.Sprintf("Session: `%s`", item.CLI.SessionID),
		fmt.Sprintf("Command: `%s`", item.CLI.Command),
	}
	if item.CLI.ExitCode != nil {
		lines = append(lines, fmt.Sprintf("Exit code: `%d`", *item.CLI.ExitCode))
	}
	if strings.TrimSpace(item.CLI.TranscriptPreview) != "" {
		lines = append(lines, fmt.Sprintf("Transcript preview: `%s`", item.CLI.TranscriptPreview))
	}
	return lines
}

func desktopSummaryLines(item Evidence) []string {
	if item.Desktop == nil {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Tool: `%s`", item.Desktop.Tool),
		fmt.Sprintf("Session: `%s`", item.Desktop.SessionID),
		fmt.Sprintf("App: `%s`", item.Desktop.AppName),
	}
	if strings.TrimSpace(item.Desktop.BundleID) != "" {
		lines = append(lines, fmt.Sprintf("Bundle ID: `%s`", item.Desktop.BundleID))
	}
	if strings.TrimSpace(item.Desktop.WindowTitle) != "" {
		lines = append(lines, fmt.Sprintf("Window title: `%s`", item.Desktop.WindowTitle))
	}
	if strings.TrimSpace(item.Desktop.State) != "" {
		lines = append(lines, fmt.Sprintf("State: `%s`", item.Desktop.State))
	}
	lines = append(lines, fmt.Sprintf("Issues: crashes=%d, fatal_logs=%d", item.Desktop.Issues.Crashes, item.Desktop.Issues.FatalLogs))
	return lines
}

func reportStatus(eval Evaluation) string {
	if eval.Complete {
		return "PASS"
	}
	return "INCOMPLETE"
}

func scenarioComplete(eval ScenarioEvaluation) bool {
	for _, surface := range eval.Scenario.RequiredSurfaces() {
		ok, exists := eval.SurfaceStatus(surface)
		if exists && !ok {
			return false
		}
	}
	return true
}

func curlModeLabel(run Run) string {
	requiredScenarios := curlRequirementRows(run)
	if len(requiredScenarios) > 0 {
		return fmt.Sprintf("required for %d scenario(s)", len(requiredScenarios))
	}
	if strings.TrimSpace(run.CurlSkipReason) == "" {
		return "skipped"
	}
	return "skipped (" + run.CurlSkipReason + ")"
}

func scenarioTone(eval ScenarioEvaluation) string {
	if scenarioComplete(eval) {
		return "ok"
	}
	return "bad"
}

func passedScenarioCount(items []scenarioHTMLReport) int {
	count := 0
	for _, item := range items {
		if scenarioComplete(item.Eval) {
			count++
		}
	}
	return count
}

func failedScenarioCount(items []scenarioHTMLReport) int {
	count := 0
	for _, item := range items {
		if !scenarioComplete(item.Eval) {
			count++
		}
	}
	return count
}

func totalScenarioCount(items []scenarioHTMLReport) int {
	return len(items)
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

// truncateActual shortens long assertion actual values for display.
func truncateActual(s string) string {
	if len(s) <= 120 {
		return s
	}
	return s[:120] + "…"
}

// allAssertionsPass returns true if every assertion in the slice passed.
func allAssertionsPass(assertions []Assertion) bool {
	for _, a := range assertions {
		if a.Result != AssertionPass {
			return false
		}
	}
	return true
}

func scenarioSurfaces(scenario Scenario) []string {
	return scenario.RequiredSurfaces()
}

func surfaceOK(eval ScenarioEvaluation, surface string) bool {
	ok, _ := eval.SurfaceStatus(surface)
	return ok
}

func surfaceIssues(eval ScenarioEvaluation, surface string) []string {
	return eval.SurfaceIssues(surface)
}

func surfaceTitle(surface string) string {
	switch surface {
	case SurfaceIOS:
		return "iOS"
	case SurfaceCLI:
		return "CLI"
	default:
		return titleCase(surface)
	}
}

func runHasHTTPSummary(run Run) bool {
	p := normalizePlatform(run.Platform)
	return p != PlatformCLI
}

func runSurfaceLabel(run Run) string {
	return surfaceTitle(normalizePlatform(run.Platform))
}

func runTargetLabel(run Run) string {
	switch normalizePlatform(run.Platform) {
	case PlatformIOS:
		return "iOS Target"
	case PlatformCLI:
		return "CLI Command"
	case PlatformDesktop:
		return "Desktop App"
	default:
		return "Browser URL"
	}
}

func runTargetValue(run Run) string {
	switch normalizePlatform(run.Platform) {
	case PlatformIOS:
		target := firstNonEmpty(run.IOS.Scheme, run.IOS.BundleID)
		if strings.TrimSpace(run.IOS.Scheme) != "" && strings.TrimSpace(run.IOS.BundleID) != "" {
			target = run.IOS.Scheme + " / " + run.IOS.BundleID
		}
		if strings.TrimSpace(run.IOS.Simulator) != "" {
			target = target + " @ " + run.IOS.Simulator
		}
		return strings.TrimSpace(target)
	case PlatformCLI:
		return run.CLICommand
	case PlatformDesktop:
		target := run.Desktop.Name
		if strings.TrimSpace(run.Desktop.BundleID) != "" {
			target = target + " (" + run.Desktop.BundleID + ")"
		}
		return strings.TrimSpace(target)
	default:
		return run.BrowserURL
	}
}

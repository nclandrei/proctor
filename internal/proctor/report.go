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
)

type scenarioReport struct {
	Eval     ScenarioEvaluation
	Evidence []Evidence
}

type scenarioHTMLReport struct {
	Eval     ScenarioEvaluation
	Evidence []evidenceHTMLReport
}

type evidenceHTMLReport struct {
	Surface    string
	Summary    []string
	Assertions []Assertion
	Artifacts  []artifactHTMLReport
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

func RenderReports(run Run, runDir string, eval Evaluation, evidence []Evidence) (string, string, error) {
	scenarios := groupEvidenceByScenario(eval, evidence)
	htmlScenarios := buildScenarioHTMLReports(runDir, scenarios)
	edgeCoverage := edgeCoverageRows(run)
	curlRequirements := curlRequirementRows(run)

	var md strings.Builder
	md.WriteString(fmt.Sprintf("# %s\n\n", run.Feature))
	md.WriteString(fmt.Sprintf("- Run ID: `%s`\n", run.ID))
	md.WriteString(fmt.Sprintf("- Platform: `%s`\n", normalizePlatform(run.Platform)))
	switch normalizePlatform(run.Platform) {
	case PlatformIOS:
		md.WriteString(fmt.Sprintf("- iOS scheme: `%s`\n", run.IOS.Scheme))
		md.WriteString(fmt.Sprintf("- iOS bundle ID: `%s`\n", run.IOS.BundleID))
		if strings.TrimSpace(run.IOS.Simulator) != "" {
			md.WriteString(fmt.Sprintf("- Simulator: `%s`\n", run.IOS.Simulator))
		}
	default:
		md.WriteString(fmt.Sprintf("- Browser URL: `%s`\n", run.BrowserURL))
	}
	if len(curlRequirements) > 0 {
		md.WriteString(fmt.Sprintf("- Direct HTTP verification: required for %d scenario(s)\n", len(curlRequirements)))
	} else if strings.TrimSpace(run.CurlSkipReason) != "" {
		md.WriteString(fmt.Sprintf("- Direct HTTP verification: skipped (%s)\n", run.CurlSkipReason))
	} else {
		md.WriteString("- Direct HTTP verification: skipped\n")
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
		md.WriteString(fmt.Sprintf("### %s\n\n", scenario.Eval.Scenario.Label))
		md.WriteString(fmt.Sprintf("- Scenario ID: `%s`\n", scenario.Eval.Scenario.ID))
		if scenario.Eval.Scenario.CurlRequired && len(scenario.Eval.Scenario.CurlEndpoints) > 0 {
			md.WriteString(fmt.Sprintf("- curl contract: `%s`\n", strings.Join(scenario.Eval.Scenario.CurlEndpoints, "`, `")))
		}
		if scenario.Eval.EvalHasBrowser() {
			if scenario.Eval.BrowserOK {
				md.WriteString("- Browser: pass\n")
			} else {
				md.WriteString(fmt.Sprintf("- Browser: fail (%s)\n", strings.Join(scenario.Eval.BrowserIssues, ", ")))
			}
		}
		if scenario.Eval.EvalHasIOS() {
			if scenario.Eval.IOSOK {
				md.WriteString("- iOS: pass\n")
			} else {
				md.WriteString(fmt.Sprintf("- iOS: fail (%s)\n", strings.Join(scenario.Eval.IOSIssues, ", ")))
			}
		}
		if scenario.Eval.Scenario.CurlRequired {
			if scenario.Eval.CurlOK {
				md.WriteString("- curl: pass\n")
			} else {
				md.WriteString(fmt.Sprintf("- curl: fail (%s)\n", strings.Join(scenario.Eval.CurlIssues, ", ")))
			}
		}

		for _, item := range scenario.Evidence {
			md.WriteString(fmt.Sprintf("\n#### %s Evidence\n\n", strings.Title(item.Surface)))
			for _, line := range evidenceSummaryLines(item) {
				md.WriteString(fmt.Sprintf("- %s\n", line))
			}
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
			for _, artifact := range item.Artifacts {
				md.WriteString(fmt.Sprintf("- Artifact: [%s](%s)\n", artifact.Label, artifact.Path))
				if artifact.Kind == ArtifactImage {
					md.WriteString(fmt.Sprintf("  \n  ![%s](%s)\n", artifact.Label, artifact.Path))
				}
			}
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
		"title":                strings.Title,
		"evidenceSummaryLines": evidenceSummaryLines,
		"reportStatus":         reportStatus,
		"scenarioComplete":     scenarioComplete,
		"scenarioTone":         scenarioTone,
		"passedScenarioCount":  passedScenarioCount,
		"failedScenarioCount":  failedScenarioCount,
		"curlModeLabel":        curlModeLabel,
		"runTargetLabel":       runTargetLabel,
		"runTargetValue":       runTargetValue,
	}).Parse(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="color-scheme" content="dark">
  <title>Proctor report - {{ .Run.Feature }}</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #081017;
      --panel: rgba(11, 20, 31, 0.86);
      --panel-strong: rgba(8, 16, 23, 0.96);
      --ink: #f5f3ef;
      --muted: #a8b0bb;
      --line: rgba(148, 163, 184, 0.18);
      --ok: #71f7b8;
      --ok-bg: rgba(34, 197, 94, 0.16);
      --bad: #ff8d8d;
      --bad-bg: rgba(239, 68, 68, 0.16);
      --accent: #7dd3fc;
      --accent-soft: rgba(34, 211, 238, 0.14);
      --shadow: 0 24px 60px rgba(0, 0, 0, 0.32);
    }
    * { box-sizing: border-box; }
    html { background: var(--bg); }
    body {
      margin: 0;
      background:
        radial-gradient(circle at top left, rgba(56, 189, 248, 0.16), transparent 28%),
        radial-gradient(circle at top right, rgba(251, 191, 36, 0.10), transparent 22%),
        linear-gradient(180deg, #09131d 0%, var(--bg) 100%);
      color: var(--ink);
      font-family: "Iowan Old Style", "Palatino Linotype", "Book Antiqua", Georgia, serif;
      line-height: 1.55;
    }
    code {
      background: rgba(148, 163, 184, 0.12);
      padding: 2px 6px;
      border-radius: 999px;
      font-family: "SFMono-Regular", Menlo, Consolas, monospace;
      font-size: 0.92em;
    }
    a { color: var(--accent); }
    .page {
      max-width: 1180px;
      margin: 0 auto;
      padding: 28px;
    }
    .hero {
      background: linear-gradient(135deg, rgba(9, 20, 31, 0.98), rgba(15, 23, 42, 0.92));
      border: 1px solid rgba(125, 211, 252, 0.14);
      border-radius: 28px;
      padding: 28px;
      box-shadow: var(--shadow);
      margin-bottom: 24px;
    }
    .eyebrow {
      display: inline-block;
      margin-bottom: 12px;
      padding: 6px 12px;
      border-radius: 999px;
      background: var(--accent-soft);
      color: var(--accent);
      font-family: "SFMono-Regular", Menlo, Consolas, monospace;
      font-size: 0.82rem;
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .hero-top {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: flex-start;
      flex-wrap: wrap;
    }
    h1, h2, h3, h4 {
      font-family: "Avenir Next", "Segoe UI", sans-serif;
      letter-spacing: -0.02em;
      margin: 0;
    }
    figure { margin: 0; }
    h1 { font-size: clamp(2rem, 4vw, 3.2rem); line-height: 1.02; margin-bottom: 10px; }
    h2 { font-size: 1.35rem; margin-bottom: 14px; }
    h3 { font-size: 1.05rem; margin-bottom: 6px; }
    h4 { font-size: 0.98rem; margin-bottom: 10px; }
    p { margin: 0; }
    .lead {
      max-width: 70ch;
      color: var(--muted);
      font-size: 1.02rem;
    }
    .status-pill,
    .surface-pill,
    .kind-pill {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 7px 12px;
      border-radius: 999px;
      font-family: "SFMono-Regular", Menlo, Consolas, monospace;
      font-size: 0.84rem;
    }
    .status-pill.ok,
    .surface-pill.ok { color: var(--ok); background: var(--ok-bg); }
    .status-pill.bad,
    .surface-pill.bad { color: var(--bad); background: var(--bad-bg); }
    .kind-pill { color: var(--accent); background: rgba(15, 118, 110, 0.08); }
    .scenario-pill {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 8px 12px;
      border-radius: 999px;
      font-family: "SFMono-Regular", Menlo, Consolas, monospace;
      font-size: 0.8rem;
      border: 1px solid transparent;
    }
    .scenario-pill.ok {
      color: var(--ok);
      background: rgba(34, 197, 94, 0.12);
      border-color: rgba(113, 247, 184, 0.22);
    }
    .scenario-pill.bad {
      color: var(--bad);
      background: rgba(239, 68, 68, 0.12);
      border-color: rgba(255, 141, 141, 0.2);
    }
    .summary-grid,
    .coverage-grid,
    .scenario-grid {
      display: grid;
      gap: 16px;
    }
    .summary-grid { grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); margin-top: 22px; }
    .coverage-grid,
    .scenario-grid { grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); }
    .summary-card,
    .coverage-card,
    .scenario-card,
    .evidence-block,
    .global-gaps {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 22px;
      box-shadow: var(--shadow);
    }
    .summary-card,
    .coverage-card,
    .global-gaps { padding: 18px; }
    .summary-label {
      font-family: "SFMono-Regular", Menlo, Consolas, monospace;
      font-size: 0.8rem;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0.05em;
      margin-bottom: 10px;
    }
    .summary-value {
      font-family: "Avenir Next", "Segoe UI", sans-serif;
      font-size: 1.02rem;
      word-break: break-word;
    }
    .section {
      margin-top: 28px;
    }
    .section-head {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: center;
      margin-bottom: 16px;
    }
    .section-note {
      color: var(--muted);
      font-size: 0.96rem;
    }
    .scenario-rollup {
      margin-top: 22px;
      padding-top: 22px;
      border-top: 1px solid var(--line);
    }
    .scenario-strip {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-top: 14px;
    }
    .coverage-card .status-note,
    .scenario-meta,
    .summary-list,
    .assertion-list,
    .gap-list {
      color: var(--muted);
    }
    .coverage-card ul,
    .summary-list,
    .assertion-list,
    .gap-list {
      margin: 10px 0 0;
      padding-left: 18px;
    }
    .scenario-card {
      padding: 22px;
      display: grid;
      gap: 18px;
      border-top: 6px solid rgba(113, 247, 184, 0.22);
    }
    .scenario-card.fail {
      border-top-color: rgba(255, 141, 141, 0.3);
    }
    .scenario-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: flex-start;
      flex-wrap: wrap;
    }
    .scenario-meta {
      margin-top: 8px;
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      align-items: center;
    }
    .scenario-status {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
    }
    .evidence-stack {
      display: grid;
      gap: 14px;
    }
    .evidence-block {
      padding: 16px;
      background: rgba(15, 23, 42, 0.72);
    }
    .artifacts {
      display: grid;
      gap: 14px;
      grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
      margin-top: 14px;
    }
    .artifact-card {
      border: 1px solid var(--line);
      border-radius: 18px;
      padding: 12px;
      background: rgba(8, 16, 23, 0.62);
      display: grid;
      gap: 12px;
      align-content: start;
    }
    .artifact-card.transcript-card {
      grid-column: 1 / -1;
    }
    .artifact-card-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }
    .artifact-title {
      font-weight: 600;
      word-break: break-word;
    }
    .artifact-link {
      color: var(--muted);
      font-size: 0.9rem;
      text-decoration: none;
      white-space: nowrap;
    }
    .artifact-link:hover { color: var(--accent); }
    .artifact-preview {
      display: block;
      color: inherit;
      text-decoration: none;
      cursor: zoom-in;
    }
    .artifact-frame {
      min-height: 180px;
      max-height: 220px;
      padding: 10px;
      border: 1px solid var(--line);
      border-radius: 14px;
      background: rgba(2, 6, 23, 0.9);
      display: grid;
      place-items: center;
      overflow: hidden;
    }
    .artifact-preview img,
    .lightbox-image {
      width: 100%;
      display: block;
      border: 1px solid var(--line);
      background: rgba(2, 6, 23, 0.9);
    }
    .artifact-preview img {
      height: 100%;
      max-height: 198px;
      object-fit: contain;
      object-position: top center;
      border-radius: 10px;
    }
    .artifact-preview-note,
    .artifact-note {
      color: var(--muted);
      font-size: 0.88rem;
    }
    .artifact-details {
      border: 1px solid var(--line);
      border-radius: 14px;
      background: rgba(2, 6, 23, 0.9);
      overflow: hidden;
    }
    .artifact-summary {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
      padding: 12px 14px;
      cursor: pointer;
      color: var(--muted);
      font-size: 0.9rem;
    }
    .artifact-summary-meta {
      font-family: "SFMono-Regular", Menlo, Consolas, monospace;
      font-size: 0.8rem;
      color: var(--accent);
      white-space: nowrap;
    }
    .artifact-log {
      margin: 0;
      padding: 14px;
      border-top: 1px solid var(--line);
      max-height: 320px;
      overflow: auto;
      background: rgba(2, 6, 23, 0.94);
      color: var(--ink);
      font-family: "SFMono-Regular", Menlo, Consolas, monospace;
      font-size: 0.85rem;
      line-height: 1.55;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .lightbox {
      position: fixed;
      inset: 0;
      display: none;
      padding: 16px;
      z-index: 999;
    }
    .lightbox:target {
      display: grid;
      place-items: center;
    }
    .lightbox-backdrop {
      position: absolute;
      inset: 0;
      background: rgba(2, 6, 23, 0.84);
      backdrop-filter: blur(10px);
    }
    .lightbox-panel {
      position: relative;
      z-index: 1;
      width: min(1100px, calc(100vw - 32px));
      max-height: calc(100vh - 32px);
      overflow: auto;
      margin: 0;
      padding: 18px;
      border: 1px solid rgba(125, 211, 252, 0.18);
      border-radius: 24px;
      background: rgba(8, 16, 23, 0.96);
      box-shadow: var(--shadow);
    }
    .lightbox-panel figcaption { font-weight: 600; }
    .lightbox-top {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: center;
      margin-bottom: 14px;
    }
    .lightbox-close {
      display: inline-flex;
      align-items: center;
      padding: 7px 12px;
      border-radius: 999px;
      color: var(--ink);
      background: rgba(148, 163, 184, 0.14);
      text-decoration: none;
    }
    .lightbox-image {
      height: auto;
      border-radius: 18px;
    }
    .assertion-list li {
      margin-bottom: 10px;
    }
    .assertion-list li.pass { color: var(--ok); }
    .assertion-list li.fail { color: var(--bad); }
    .inline-detail {
      display: block;
      margin-top: 4px;
    }
    .global-gaps { margin-top: 26px; }
    @media (max-width: 720px) {
      .page { padding: 16px; }
      .hero,
      .scenario-card,
      .summary-card,
      .coverage-card,
      .global-gaps { border-radius: 18px; }
      .summary-grid,
      .coverage-grid,
      .scenario-grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main class="page">
    <section class="hero">
      <span class="eyebrow">Proctor Report</span>
      <div class="hero-top">
        <div>
          <h1>{{ .Run.Feature }}</h1>
          <p class="lead">Manual verification contract rendered from recorded evidence. The report is human-facing; the raw artifacts remain the source of truth.</p>
        </div>
        <div class="status-pill {{ if .Eval.Complete }}ok{{ else }}bad{{ end }}">{{ reportStatus .Eval }}</div>
      </div>
      <div class="summary-grid">
        <article class="summary-card">
          <div class="summary-label">Run ID</div>
          <div class="summary-value"><code>{{ .Run.ID }}</code></div>
        </article>
        <article class="summary-card">
          <div class="summary-label">{{ runTargetLabel .Run }}</div>
          <div class="summary-value"><code>{{ runTargetValue .Run }}</code></div>
        </article>
        <article class="summary-card">
          <div class="summary-label">Direct HTTP Verification</div>
          <div class="summary-value">{{ curlModeLabel .Run }}</div>
        </article>
        <article class="summary-card">
          <div class="summary-label">Scenario Count</div>
          <div class="summary-value">{{ len .Scenarios }} scenario(s)</div>
        </article>
      </div>
      <section class="scenario-rollup">
        <div class="section-head">
          <div>
            <h2>Scenario Rollup</h2>
            <div class="section-note">Compact pass/fail summary before the full evidence breakdown.</div>
          </div>
        </div>
        <div class="summary-grid">
          <article class="summary-card">
            <div class="summary-label">Passed Scenarios</div>
            <div class="summary-value">{{ passedScenarioCount .Scenarios }}</div>
          </article>
          <article class="summary-card">
            <div class="summary-label">Failed Scenarios</div>
            <div class="summary-value">{{ failedScenarioCount .Scenarios }}</div>
          </article>
        </div>
        <div class="scenario-strip">
          {{ range .Scenarios }}
          <span class="scenario-pill {{ scenarioTone .Eval }}">{{ .Eval.Scenario.Label }}</span>
          {{ end }}
        </div>
      </section>
	      {{ if .CurlCoverage }}
	      <div class="section" style="margin-top: 20px;">
	        <div class="summary-label">HTTP Risk Coverage</div>
	        <ul class="summary-list">
	          {{ range .CurlCoverage }}
	          <li>
	            <strong>{{ .Label }}</strong>
	            <span class="inline-detail"><code>{{ .ID }}</code></span>
	            {{ if .Endpoints }}
	            <span class="inline-detail">{{ range $index, $endpoint := .Endpoints }}{{ if $index }}, {{ end }}<code>{{ $endpoint }}</code>{{ end }}</span>
	            {{ end }}
	          </li>
	          {{ end }}
	        </ul>
	      </div>
	      {{ end }}
    </section>

    {{ if .EdgeCoverage }}
    <section class="section">
      <div class="section-head">
        <div>
          <h2>Edge Case Coverage</h2>
          <div class="section-note">Every category is either represented by concrete scenarios or explicitly marked not applicable.</div>
        </div>
      </div>
      <div class="coverage-grid">
        {{ range .EdgeCoverage }}
        <article class="coverage-card">
          <h3>{{ .Category }}</h3>
          {{ if eq .Status "na" }}
          <div class="status-note">Status: N/A ({{ .Reason }})</div>
          {{ else }}
          <div class="status-note">Status: scenario coverage required</div>
          <ul>{{ range .ScenarioLabels }}<li>{{ . }}</li>{{ end }}</ul>
          {{ end }}
        </article>
        {{ end }}
      </div>
    </section>
    {{ end }}

    <section class="section">
      <div class="section-head">
        <div>
          <h2>Scenarios</h2>
          <div class="section-note">Each scenario below shows the contract result plus the concrete evidence Proctor recorded.</div>
        </div>
      </div>
      <div class="scenario-grid">
        {{ range .Scenarios }}
        <article class="scenario-card {{ if scenarioComplete .Eval }}pass{{ else }}fail{{ end }}">
	          <div class="scenario-head">
            <div>
              <h3>{{ .Eval.Scenario.Label }}</h3>
              <div class="scenario-meta">
                <span class="kind-pill">{{ title .Eval.Scenario.Kind }}</span>
                <code>{{ .Eval.Scenario.ID }}</code>
              </div>
            </div>
            <div class="scenario-status">
              {{ if .Eval.EvalHasBrowser }}{{ if .Eval.BrowserOK }}<span class="surface-pill ok">browser: pass</span>{{ else }}<span class="surface-pill bad">browser: fail</span>{{ end }}{{ end }}
              {{ if .Eval.EvalHasIOS }}{{ if .Eval.IOSOK }}<span class="surface-pill ok">ios: pass</span>{{ else }}<span class="surface-pill bad">ios: fail</span>{{ end }}{{ end }}
              {{ if .Eval.Scenario.CurlRequired }}{{ if .Eval.CurlOK }}<span class="surface-pill ok">curl: pass</span>{{ else }}<span class="surface-pill bad">curl: fail</span>{{ end }}{{ end }}
            </div>
	          </div>
	          {{ if and .Eval.Scenario.CurlRequired .Eval.Scenario.CurlEndpoints }}
	          <div class="status-note">curl contract: {{ range $index, $endpoint := .Eval.Scenario.CurlEndpoints }}{{ if $index }}, {{ end }}<code>{{ $endpoint }}</code>{{ end }}</div>
	          {{ end }}
	          {{ if and .Eval.EvalHasBrowser (not .Eval.BrowserOK) }}
	          <ul class="gap-list">{{ range .Eval.BrowserIssues }}<li>{{ . }}</li>{{ end }}</ul>
          {{ end }}
          {{ if and .Eval.EvalHasIOS (not .Eval.IOSOK) }}
          <ul class="gap-list">{{ range .Eval.IOSIssues }}<li>{{ . }}</li>{{ end }}</ul>
          {{ end }}
          {{ if and .Eval.Scenario.CurlRequired (not .Eval.CurlOK) }}
          <ul class="gap-list">{{ range .Eval.CurlIssues }}<li>{{ . }}</li>{{ end }}</ul>
          {{ end }}
          <div class="evidence-stack">
            {{ range .Evidence }}
            <section class="evidence-block">
              <h4>{{ title .Surface }} evidence</h4>
              {{ if .Summary }}
              <ul class="summary-list">
                {{ range .Summary }}
                <li>{{ . }}</li>
                {{ end }}
              </ul>
              {{ end }}
              <ul class="assertion-list">
                {{ range .Assertions }}
                <li class="{{ if eq .Result "pass" }}pass{{ else }}fail{{ end }}">
                  <code>{{ .Description }}</code>
                  {{ if or .Expected .Actual }}
                  <span class="inline-detail">expected: <code>{{ .Expected }}</code></span>
                  <span class="inline-detail">actual: <code>{{ .Actual }}</code></span>
                  {{ end }}
                  {{ if .Message }}
                  <span class="inline-detail">{{ .Message }}</span>
                  {{ end }}
                </li>
                {{ end }}
              </ul>
              <div class="artifacts">
                {{ range .Artifacts }}
                <article class="artifact-card{{ if .HasInlineText }} transcript-card{{ end }}">
                  <div class="artifact-card-head">
                    <div class="artifact-title">{{ .Label }}</div>
                    <a class="artifact-link" href="{{ .Path }}">artifact</a>
                  </div>
                  {{ if .InlineSource }}
                  <a class="artifact-preview" href="#{{ .ModalID }}">
                    <div class="artifact-frame">
                      <img src="{{ .InlineSource }}" alt="{{ .Label }}">
                    </div>
                    <div class="artifact-preview-note">Embedded preview. Click to enlarge.</div>
                  </a>
                  <div class="lightbox" id="{{ .ModalID }}">
                    <a class="lightbox-backdrop" href="#" aria-label="Close enlarged preview"></a>
                    <figure class="lightbox-panel">
                      <div class="lightbox-top">
                        <figcaption>{{ .Label }}</figcaption>
                        <a class="lightbox-close" href="#">Close</a>
                      </div>
                      <img class="lightbox-image" src="{{ .InlineSource }}" alt="{{ .Label }}">
                    </figure>
                  </div>
                  {{ else if .HasInlineText }}
                  <details class="artifact-details">
                    <summary class="artifact-summary">
                      <span>Embedded log transcript. Expand to inspect.</span>
                      <span class="artifact-summary-meta">{{ if gt .InlineTextLineCount 0 }}{{ .InlineTextLineCount }} line(s){{ else }}empty{{ end }}</span>
                    </summary>
                    <pre class="artifact-log">{{ .InlineText }}</pre>
                  </details>
                  {{ else }}
                  <div class="artifact-note">Linked artifact</div>
                  {{ end }}
                </article>
                {{ end }}
              </div>
            </section>
            {{ end }}
          </div>
        </article>
        {{ end }}
      </div>
    </section>

    {{ if .Eval.GlobalMissing }}
    <section class="global-gaps">
      <h2>Global Gaps</h2>
      <ul class="gap-list">
        {{ range .Eval.GlobalMissing }}<li>{{ . }}</li>{{ end }}
      </ul>
    </section>
    {{ end }}
  </main>
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

func (s ScenarioEvaluation) EvalHasBrowser() bool {
	return s.Scenario.BrowserRequired
}

func (s ScenarioEvaluation) EvalHasIOS() bool {
	return s.Scenario.IOSRequired
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

func buildScenarioHTMLReports(runDir string, scenarios []scenarioReport) []scenarioHTMLReport {
	reports := make([]scenarioHTMLReport, 0, len(scenarios))
	for scenarioIdx, scenario := range scenarios {
		htmlScenario := scenarioHTMLReport{Eval: scenario.Eval}
		for evidenceIdx, item := range scenario.Evidence {
			htmlEvidence := evidenceHTMLReport{
				Surface:    item.Surface,
				Summary:    evidenceSummaryLines(item),
				Assertions: append([]Assertion(nil), item.Assertions...),
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

func reportStatus(eval Evaluation) string {
	if eval.Complete {
		return "PASS"
	}
	return "INCOMPLETE"
}

func scenarioComplete(eval ScenarioEvaluation) bool {
	browserOK := !eval.Scenario.BrowserRequired || eval.BrowserOK
	iosOK := !eval.Scenario.IOSRequired || eval.IOSOK
	curlOK := !eval.Scenario.CurlRequired || eval.CurlOK
	return browserOK && iosOK && curlOK
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

func runTargetLabel(run Run) string {
	switch normalizePlatform(run.Platform) {
	case PlatformIOS:
		return "iOS Target"
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
	default:
		return run.BrowserURL
	}
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

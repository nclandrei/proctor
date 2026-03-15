package proctor

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
)

type scenarioReport struct {
	Eval     ScenarioEvaluation
	Evidence []Evidence
}

func RenderReports(run Run, eval Evaluation, evidence []Evidence) (string, string, error) {
	scenarios := groupEvidenceByScenario(eval, evidence)
	edgeCoverage := edgeCoverageRows(run)

	var md strings.Builder
	md.WriteString(fmt.Sprintf("# %s\n\n", run.Feature))
	md.WriteString(fmt.Sprintf("- Run ID: `%s`\n", run.ID))
	md.WriteString(fmt.Sprintf("- Browser URL: `%s`\n", run.BrowserURL))
	if run.CurlRequired {
		md.WriteString("- Direct HTTP verification: required\n")
	} else {
		md.WriteString(fmt.Sprintf("- Direct HTTP verification: skipped (%s)\n", run.CurlSkipReason))
	}
	if len(run.CurlEndpoints) > 0 {
		md.WriteString("- Endpoints:\n")
		for _, endpoint := range run.CurlEndpoints {
			md.WriteString(fmt.Sprintf("  - `%s`\n", endpoint))
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
		if scenario.Eval.EvalHasBrowser() {
			if scenario.Eval.BrowserOK {
				md.WriteString("- Browser: pass\n")
			} else {
				md.WriteString(fmt.Sprintf("- Browser: fail (%s)\n", strings.Join(scenario.Eval.BrowserIssues, ", ")))
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
		Scenarios    []scenarioReport
		EdgeCoverage []edgeCoverageRow
	}
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"join":                 strings.Join,
		"title":                strings.Title,
		"evidenceSummaryLines": evidenceSummaryLines,
		"reportStatus":         reportStatus,
		"scenarioComplete":     scenarioComplete,
		"curlModeLabel":        curlModeLabel,
	}).Parse(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Proctor report - {{ .Run.Feature }}</title>
  <style>
    :root {
      --bg: #f6f0e5;
      --panel: rgba(255, 252, 245, 0.88);
      --ink: #1c1917;
      --muted: #57534e;
      --line: #d6d3d1;
      --ok: #166534;
      --ok-bg: #dcfce7;
      --bad: #991b1b;
      --bad-bg: #fee2e2;
      --accent: #0f766e;
      --accent-soft: #ccfbf1;
      --shadow: 0 18px 48px rgba(28, 25, 23, 0.08);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background:
        radial-gradient(circle at top left, rgba(15, 118, 110, 0.10), transparent 30%),
        linear-gradient(180deg, #faf7f0 0%, var(--bg) 100%);
      color: var(--ink);
      font-family: "Iowan Old Style", "Palatino Linotype", "Book Antiqua", Georgia, serif;
      line-height: 1.55;
    }
    code {
      background: rgba(28, 25, 23, 0.06);
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
      background: linear-gradient(135deg, rgba(255, 252, 245, 0.98), rgba(255, 247, 237, 0.92));
      border: 1px solid rgba(214, 211, 209, 0.8);
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
      border: 1px solid rgba(214, 211, 209, 0.9);
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
      border-top: 6px solid rgba(15, 118, 110, 0.18);
    }
    .scenario-card.fail {
      border-top-color: rgba(153, 27, 27, 0.34);
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
      background: rgba(255, 255, 255, 0.72);
    }
    .artifacts {
      display: grid;
      gap: 14px;
      grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
      margin-top: 14px;
    }
    .artifact-card {
      border: 1px solid rgba(214, 211, 209, 0.9);
      border-radius: 18px;
      padding: 12px;
      background: rgba(255, 255, 255, 0.78);
    }
    img {
      width: 100%;
      display: block;
      margin-top: 10px;
      border: 1px solid rgba(214, 211, 209, 0.9);
      border-radius: 12px;
      background: white;
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
          <div class="summary-label">Browser URL</div>
          <div class="summary-value"><code>{{ .Run.BrowserURL }}</code></div>
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
      {{ if .Run.CurlEndpoints }}
      <div class="section" style="margin-top: 20px;">
        <div class="summary-label">Endpoints</div>
        <ul class="summary-list">{{ range .Run.CurlEndpoints }}<li><code>{{ . }}</code></li>{{ end }}</ul>
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
              {{ if .Eval.Scenario.CurlRequired }}{{ if .Eval.CurlOK }}<span class="surface-pill ok">curl: pass</span>{{ else }}<span class="surface-pill bad">curl: fail</span>{{ end }}{{ end }}
            </div>
          </div>
          {{ if and .Eval.EvalHasBrowser (not .Eval.BrowserOK) }}
          <ul class="gap-list">{{ range .Eval.BrowserIssues }}<li>{{ . }}</li>{{ end }}</ul>
          {{ end }}
          {{ if and .Eval.Scenario.CurlRequired (not .Eval.CurlOK) }}
          <ul class="gap-list">{{ range .Eval.CurlIssues }}<li>{{ . }}</li>{{ end }}</ul>
          {{ end }}
          <div class="evidence-stack">
            {{ range .Evidence }}
            <section class="evidence-block">
              <h4>{{ title .Surface }} evidence</h4>
              {{ $summary := evidenceSummaryLines . }}
              {{ if $summary }}
              <ul class="summary-list">
                {{ range $summary }}
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
                </li>
                {{ end }}
              </ul>
              <div class="artifacts">
                {{ range .Artifacts }}
                <article class="artifact-card">
                  <div><a href="{{ .Path }}">{{ .Label }}</a></div>
                  {{ if eq .Kind "image" }}<img src="{{ .Path }}" alt="{{ .Label }}">{{ end }}
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
	if err := tmpl.Execute(&html, reportData{Run: run, Eval: eval, Scenarios: scenarios, EdgeCoverage: edgeCoverage}); err != nil {
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

type edgeCoverageRow struct {
	Category       string
	Status         string
	Reason         string
	ScenarioLabels []string
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

func evidenceSummaryLines(item Evidence) []string {
	switch item.Surface {
	case SurfaceBrowser:
		return browserSummaryLines(item)
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
	curlOK := !eval.Scenario.CurlRequired || eval.CurlOK
	return browserOK && curlOK
}

func curlModeLabel(run Run) string {
	if run.CurlRequired {
		return "required"
	}
	return "skipped (" + run.CurlSkipReason + ")"
}

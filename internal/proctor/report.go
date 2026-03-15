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
		"join":  strings.Join,
		"title": strings.Title,
	}).Parse(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Proctor report - {{ .Run.Feature }}</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 32px; line-height: 1.5; color: #111827; }
    code { background: #f3f4f6; padding: 2px 4px; border-radius: 4px; }
    .ok { color: #166534; }
    .bad { color: #b91c1c; }
    .artifacts { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); }
    img { max-width: 100%; border: 1px solid #d1d5db; border-radius: 8px; }
    .card { border: 1px solid #e5e7eb; border-radius: 12px; padding: 16px; margin-bottom: 20px; }
  </style>
</head>
<body>
  <h1>{{ .Run.Feature }}</h1>
  <p><strong>Run ID:</strong> <code>{{ .Run.ID }}</code></p>
  <p><strong>Browser URL:</strong> <code>{{ .Run.BrowserURL }}</code></p>
  {{ if .Run.CurlRequired }}
  <p><strong>Direct HTTP verification:</strong> required</p>
  {{ else }}
  <p><strong>Direct HTTP verification:</strong> skipped ({{ .Run.CurlSkipReason }})</p>
  {{ end }}
  {{ if .Run.CurlEndpoints }}
  <p><strong>Endpoints:</strong></p>
  <ul>{{ range .Run.CurlEndpoints }}<li><code>{{ . }}</code></li>{{ end }}</ul>
  {{ end }}
  {{ if .EdgeCoverage }}
  <h2>Edge Case Coverage</h2>
  {{ range .EdgeCoverage }}
  <section class="card">
    <h3>{{ .Category }}</h3>
    {{ if eq .Status "na" }}
    <div>Status: N/A ({{ .Reason }})</div>
    {{ else }}
    <div>Status: scenario coverage required</div>
    <ul>{{ range .ScenarioLabels }}<li>{{ . }}</li>{{ end }}</ul>
    {{ end }}
  </section>
  {{ end }}
  {{ end }}
  <h2>Scenarios</h2>
  {{ range .Scenarios }}
  <section class="card">
    <h3>{{ .Eval.Scenario.Label }}</h3>
    <p><code>{{ .Eval.Scenario.ID }}</code></p>
    {{ if .Eval.EvalHasBrowser }}{{ if .Eval.BrowserOK }}<div class="ok">browser: pass</div>{{ else }}<div class="bad">browser: fail ({{ join .Eval.BrowserIssues ", " }})</div>{{ end }}{{ end }}
    {{ if .Eval.Scenario.CurlRequired }}{{ if .Eval.CurlOK }}<div class="ok">curl: pass</div>{{ else }}<div class="bad">curl: fail ({{ join .Eval.CurlIssues ", " }})</div>{{ end }}{{ end }}
    {{ range .Evidence }}
    <h4>{{ title .Surface }} evidence</h4>
    <ul>
      {{ range .Assertions }}
      <li class="{{ if eq .Result "pass" }}ok{{ else }}bad{{ end }}">
        <code>{{ .Description }}</code>
        {{ if or .Expected .Actual }}
        <div>expected: <code>{{ .Expected }}</code></div>
        <div>actual: <code>{{ .Actual }}</code></div>
        {{ end }}
      </li>
      {{ end }}
    </ul>
    <div class="artifacts">
      {{ range .Artifacts }}
      <div>
        <div><a href="{{ .Path }}">{{ .Label }}</a></div>
        {{ if eq .Kind "image" }}<img src="{{ .Path }}" alt="{{ .Label }}">{{ end }}
      </div>
      {{ end }}
    </div>
    {{ end }}
  </section>
  {{ end }}
  {{ if .Eval.GlobalMissing }}
  <h2>Global Gaps</h2>
  <ul>
    {{ range .Eval.GlobalMissing }}<li class="bad">{{ . }}</li>{{ end }}
  </ul>
  {{ end }}
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

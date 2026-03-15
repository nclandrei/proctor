package proctor

import (
	"fmt"
	"html/template"
	"strings"
)

func RenderReports(run Run, eval Evaluation) (string, string, error) {
	var md strings.Builder
	md.WriteString(fmt.Sprintf("# %s\n\n", run.Feature))
	md.WriteString(fmt.Sprintf("- Run ID: `%s`\n", run.ID))
	md.WriteString(fmt.Sprintf("- Browser URL: `%s`\n", run.BrowserURL))
	if run.CurlRequired {
		md.WriteString("- Direct HTTP verification: required\n")
	} else {
		md.WriteString(fmt.Sprintf("- Direct HTTP verification: skipped (%s)\n", run.CurlSkipReason))
	}
	md.WriteString("\n## Required Scenarios\n\n")
	for _, item := range eval.ScenarioEvaluations {
		md.WriteString(fmt.Sprintf("- `%s`: %s\n", item.Scenario.ID, item.Scenario.Label))
		if item.BrowserOK {
			md.WriteString("  - browser: pass\n")
		} else if item.Scenario.BrowserRequired {
			md.WriteString(fmt.Sprintf("  - browser: missing (%s)\n", strings.Join(item.BrowserIssues, ", ")))
		}
		if item.Scenario.CurlRequired {
			if item.CurlOK {
				md.WriteString("  - curl: pass\n")
			} else {
				md.WriteString(fmt.Sprintf("  - curl: missing (%s)\n", strings.Join(item.CurlIssues, ", ")))
			}
		}
	}
	if len(eval.GlobalMissing) > 0 {
		md.WriteString("\n## Global Gaps\n\n")
		for _, item := range eval.GlobalMissing {
			md.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}

	type reportData struct {
		Run  Run
		Eval Evaluation
	}
	tmpl, err := template.New("report").Funcs(template.FuncMap{"join": strings.Join}).Parse(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Proctor report - {{ .Run.Feature }}</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 32px; line-height: 1.5; }
    code { background: #f3f4f6; padding: 2px 4px; }
    .ok { color: #166534; }
    .bad { color: #b91c1c; }
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
  <h2>Scenarios</h2>
  <ul>
    {{ range .Eval.ScenarioEvaluations }}
    <li>
      <strong>{{ .Scenario.Label }}</strong> (<code>{{ .Scenario.ID }}</code>)
      {{ if .BrowserOK }}<div class="ok">browser: pass</div>{{ else if .Scenario.BrowserRequired }}<div class="bad">browser: {{ join .BrowserIssues ", " }}</div>{{ end }}
      {{ if .Scenario.CurlRequired }}{{ if .CurlOK }}<div class="ok">curl: pass</div>{{ else }}<div class="bad">curl: {{ join .CurlIssues ", " }}</div>{{ end }}{{ end }}
    </li>
    {{ end }}
  </ul>
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
	if err := tmpl.Execute(&html, reportData{Run: run, Eval: eval}); err != nil {
		return "", "", err
	}
	return md.String(), html.String(), nil
}

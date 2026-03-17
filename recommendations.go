package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/nclandrei/proctor/internal/proctor"
)

var lookPathFn = exec.LookPath

type captureTools struct {
	AgentBrowser bool
	Playwright   bool
	Chrome       bool
	Xcrun        bool
	Ghostty      bool
	Tmux         bool
	Script       bool
	Curl         bool
}

func detectCaptureTools() captureTools {
	return detectCaptureToolsWithLookPath(lookPathFn)
}

func detectCaptureToolsWithLookPath(lookPath func(string) (string, error)) captureTools {
	has := func(names ...string) bool {
		for _, name := range names {
			if _, err := lookPath(name); err == nil {
				return true
			}
		}
		return false
	}

	return captureTools{
		AgentBrowser: has("agent-browser"),
		Playwright:   has("playwright"),
		Chrome:       has("google-chrome", "chromium", "chromium-browser"),
		Xcrun:        has("xcrun"),
		Ghostty:      has("ghostty"),
		Tmux:         has("tmux"),
		Script:       has("script"),
		Curl:         has("curl"),
	}
}

func printRunRecommendations(out io.Writer, heading string, run proctor.Run, eval *proctor.Evaluation) {
	lines := runRecommendationLines(run, eval, detectCaptureTools())
	if len(lines) == 0 {
		return
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, heading)
	for _, line := range lines {
		fmt.Fprintf(out, "- %s\n", line)
	}
}

func runRecommendationLines(run proctor.Run, eval *proctor.Evaluation, tools captureTools) []string {
	platform := proctor.NormalizeRunSurface(nonEmpty(run.Platform, run.Surface))
	includeCurl := false

	switch {
	case eval == nil:
		includeCurl = platform != proctor.PlatformCLI && !strings.EqualFold(strings.TrimSpace(run.CurlMode), proctor.CurlModeSkip)
	case evalNeedsCurl(run, *eval):
		includeCurl = true
	}

	switch platform {
	case proctor.PlatformIOS:
		if eval == nil || evalNeedsIOS(*eval) {
			return platformRecommendationLines(platform, includeCurl, tools)
		}
	case proctor.PlatformCLI:
		if eval == nil || evalNeedsCLI(*eval) {
			return platformRecommendationLines(platform, false, tools)
		}
	default:
		if eval == nil || evalNeedsBrowser(*eval) {
			return platformRecommendationLines(platform, includeCurl, tools)
		}
	}

	if includeCurl {
		return []string{curlRecommendationLine(tools)}
	}
	return nil
}

func platformRecommendationLines(platform string, includeCurl bool, tools captureTools) []string {
	var lines []string
	switch proctor.NormalizeRunSurface(platform) {
	case proctor.PlatformIOS:
		lines = append(lines, iosRecommendationLine(tools))
	case proctor.PlatformCLI:
		lines = append(lines, cliRecommendationLine(tools))
	default:
		lines = append(lines, browserRecommendationLine(tools))
	}
	if includeCurl {
		lines = append(lines, curlRecommendationLine(tools))
	}
	return lines
}

func allPlatformRecommendationSection() string {
	tools := detectCaptureTools()
	var b strings.Builder
	b.WriteString("\nKnown-good capture discovery on this machine:\n")
	for _, line := range platformRecommendationLines(proctor.PlatformWeb, true, tools) {
		b.WriteString("  - " + line + "\n")
	}
	for _, line := range platformRecommendationLines(proctor.PlatformIOS, true, tools) {
		if !strings.HasPrefix(line, "HTTP:") {
			b.WriteString("  - " + line + "\n")
		}
	}
	for _, line := range platformRecommendationLines(proctor.PlatformCLI, false, tools) {
		b.WriteString("  - " + line + "\n")
	}
	b.WriteString("These are recommendations only. Proctor stays tool-agnostic and accepts any workflow that produces the required artifacts.\n")
	return b.String()
}

func platformRecommendationSection(platform string, includeCurl bool) string {
	var b strings.Builder
	b.WriteString("\nKnown-good discovery on this machine:\n")
	for _, line := range platformRecommendationLines(platform, includeCurl, detectCaptureTools()) {
		b.WriteString("  - " + line + "\n")
	}
	b.WriteString("These are recommendations only. Proctor stays tool-agnostic and accepts any workflow that produces the required artifacts.\n")
	return b.String()
}

func browserRecommendationLine(tools captureTools) string {
	switch {
	case tools.AgentBrowser:
		return "Web: `agent-browser` detected on PATH. Recommended workflow: use it to drive the real browser, capture desktop and mobile screenshots, write `report.json`, then attach with `proctor record browser`."
	case tools.Playwright:
		return "Web: `playwright` detected on PATH. Recommended workflow: use it to capture desktop and mobile screenshots and synthesize the small `report.json` Proctor expects, then attach with `proctor record browser`."
	case tools.Chrome:
		return "Web: a local browser binary is detected on PATH. Use your browser tooling to capture desktop and mobile screenshots plus `report.json`, then attach with `proctor record browser`."
	default:
		return "Web: use any browser workflow that can produce desktop and mobile screenshots plus `report.json`; Proctor only cares about the artifacts and assertions."
	}
}

func iosRecommendationLine(tools captureTools) string {
	if tools.Xcrun {
		return "iOS: `xcrun` detected on PATH. Recommended workflow: use Simulator tooling such as `xcrun simctl` to boot, screenshot, and inspect logs, write `ios-report.json`, then attach with `proctor record ios`."
	}
	return "iOS: use your simulator tooling of choice to build, screenshot, and inspect logs, then write `ios-report.json` and attach it with `proctor record ios`."
}

func cliRecommendationLine(tools captureTools) string {
	switch {
	case tools.Ghostty && tools.Tmux:
		return "CLI/TUI: `ghostty` and `tmux` are detected on PATH. Recommended workflow: keep one real terminal session alive, capture a screenshot plus transcript from that session, then attach with `proctor record cli`."
	case tools.Tmux:
		return "CLI/TUI: `tmux` is detected on PATH. Recommended workflow: run the app in a real terminal, keep the session alive with `tmux`, capture a screenshot and transcript, then attach with `proctor record cli`."
	case tools.Script:
		return "CLI/TUI: `script` is detected on PATH. Use a real terminal plus `script` or your terminal's transcript capture to record the session, then attach a screenshot and transcript with `proctor record cli`."
	default:
		return "CLI/TUI: use any real terminal workflow that can provide one screenshot, one transcript, the exercised command, and a stable session id for `proctor record cli`."
	}
}

func curlRecommendationLine(tools captureTools) string {
	if tools.Curl {
		return "HTTP: `curl` is detected on PATH. For risky scenarios, wrap the real request with `proctor record curl --scenario ... -- curl -si ...`."
	}
	return "HTTP: for risky scenarios, wrap a real HTTP request with `proctor record curl --scenario ... -- <command>` so Proctor can validate the response against the scenario contract."
}

func evalNeedsBrowser(eval proctor.Evaluation) bool {
	if sliceContainsSubstring(eval.GlobalMissing, "desktop screenshot") || sliceContainsSubstring(eval.GlobalMissing, "mobile screenshot") {
		return true
	}
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.BrowserRequired && !item.BrowserOK {
			return true
		}
	}
	return false
}

func evalNeedsIOS(eval proctor.Evaluation) bool {
	if sliceContainsSubstring(eval.GlobalMissing, "iOS screenshot") {
		return true
	}
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.IOSRequired && !item.IOSOK {
			return true
		}
	}
	return false
}

func evalNeedsCLI(eval proctor.Evaluation) bool {
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.CLIRequired && !item.CLIOK {
			return true
		}
	}
	return false
}

func evalNeedsCurl(run proctor.Run, eval proctor.Evaluation) bool {
	if strings.EqualFold(strings.TrimSpace(run.CurlMode), proctor.CurlModeSkip) {
		return false
	}
	for _, item := range eval.ScenarioEvaluations {
		if item.Scenario.CurlRequired && !item.CurlOK {
			return true
		}
	}
	return false
}

func sliceContainsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

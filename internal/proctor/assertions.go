package proctor

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Browser reports keep warnings for review, but the default completion gate only
// blocks on signals we treat as functional breakage. Teams can opt into stricter
// warning handling with explicit assertions such as "console_warnings = 0".
var defaultBlockingBrowserHealthChecks = []string{
	"console_errors",
	"page_errors",
	"failed_requests",
	"http_errors",
}

var defaultBlockingIOSHealthChecks = []string{
	"launch_errors",
	"crashes",
	"fatal_logs",
}

var defaultBlockingDesktopHealthChecks = []string{
	"crashes",
	"fatal_logs",
}

type agentBrowserAuditReport struct {
	Desktop *agentBrowserDevice `json:"desktop"`
	Mobile  *agentBrowserDevice `json:"mobile"`
}

type agentBrowserDevice struct {
	Title    string                 `json:"title"`
	FinalURL string                 `json:"finalUrl"`
	Issues   map[string]interface{} `json:"issues"`
}

type iosAuditReport struct {
	Simulator *iosAuditSimulator `json:"simulator"`
	App       *iosAuditApp       `json:"app"`
	Issues    *iosAuditIssues    `json:"issues"`
}

type iosAuditSimulator struct {
	Name    string `json:"name"`
	UDID    string `json:"udid"`
	Runtime string `json:"runtime"`
}

type iosAuditApp struct {
	BundleID   string   `json:"bundleId"`
	Screen     string   `json:"screen"`
	State      string   `json:"state"`
	LaunchArgs []string `json:"launchArgs"`
	AppLaunch  *bool    `json:"appLaunch"`
}

type iosAuditIssues struct {
	LaunchErrors int `json:"launchErrors"`
	Crashes      int `json:"crashes"`
	FatalLogs    int `json:"fatalLogs"`
}

func ParseBrowserReport(path string) (BrowserData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BrowserData{}, err
	}
	var report agentBrowserAuditReport
	if err := json.Unmarshal(data, &report); err != nil {
		return BrowserData{}, fmt.Errorf("parse browser report: %w", err)
	}
	if report.Desktop == nil {
		return BrowserData{}, fmt.Errorf("browser report is missing desktop results")
	}
	result := BrowserData{
		Desktop: summarizeBrowserDevice(*report.Desktop),
	}
	if report.Mobile != nil {
		mobile := summarizeBrowserDevice(*report.Mobile)
		result.Mobile = &mobile
	}
	return result, nil
}

func ParseIOSReport(path string) (IOSData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return IOSData{}, err
	}
	var report iosAuditReport
	if err := json.Unmarshal(data, &report); err != nil {
		return IOSData{}, fmt.Errorf("parse ios report: %w", err)
	}

	result := IOSData{}
	if report.Simulator != nil {
		result.Simulator = IOSSimulatorSummary{
			Name:    report.Simulator.Name,
			UDID:    report.Simulator.UDID,
			Runtime: report.Simulator.Runtime,
		}
	}
	if report.App != nil {
		result.BundleID = report.App.BundleID
		result.Screen = report.App.Screen
		result.State = report.App.State
		result.LaunchArgs = append([]string(nil), report.App.LaunchArgs...)
		if report.App.AppLaunch != nil {
			result.AppLaunch = *report.App.AppLaunch
		}
	}
	if report.Issues != nil {
		result.Issues = IOSIssueSummary{
			LaunchErrors: report.Issues.LaunchErrors,
			Crashes:      report.Issues.Crashes,
			FatalLogs:    report.Issues.FatalLogs,
		}
	}
	if report.App == nil || report.App.AppLaunch == nil {
		result.AppLaunch = result.Issues.LaunchErrors == 0
	}
	return result, nil
}

func summarizeBrowserDevice(device agentBrowserDevice) BrowserDeviceSummary {
	return BrowserDeviceSummary{
		Title:           device.Title,
		FinalURL:        device.FinalURL,
		ConsoleErrors:   issueCount(device.Issues, "consoleErrors"),
		ConsoleWarnings: issueCount(device.Issues, "consoleWarnings"),
		PageErrors:      issueCount(device.Issues, "pageErrors"),
		FailedRequests:  issueCount(device.Issues, "failedRequests"),
		HTTPErrors:      issueCount(device.Issues, "httpErrors"),
	}
}

func issueCount(issues map[string]interface{}, key string) int {
	raw, ok := issues[key]
	if !ok {
		return 0
	}
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func EvaluateBrowserAssertions(expressions, failingExpressions []string, data BrowserData, artifacts []Artifact) ([]Assertion, error) {
	var assertions []Assertion
	covered := coveredAssertionKeys(expressions, failingExpressions)
	for _, expression := range normalizedLines(expressions) {
		assertion, err := evaluateBrowserAssertion(expression, data, artifacts, true)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, expression := range normalizedLines(failingExpressions) {
		assertion, err := evaluateBrowserAssertion(expression, data, artifacts, false)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, assertion := range implicitBrowserAssertions(covered, data) {
		assertions = append(assertions, assertion)
	}
	return assertions, nil
}

func EvaluateIOSAssertions(expressions, failingExpressions []string, data IOSData, artifacts []Artifact) ([]Assertion, error) {
	var assertions []Assertion
	covered := coveredAssertionKeys(expressions, failingExpressions)
	for _, expression := range normalizedLines(expressions) {
		assertion, err := evaluateIOSAssertion(expression, data, artifacts, true)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, expression := range normalizedLines(failingExpressions) {
		assertion, err := evaluateIOSAssertion(expression, data, artifacts, false)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, assertion := range implicitIOSAssertions(covered, data) {
		assertions = append(assertions, assertion)
	}
	return assertions, nil
}

func evaluateBrowserAssertion(expression string, data BrowserData, artifacts []Artifact, expectPass bool) (Assertion, error) {
	left, operator, right, err := splitAssertion(expression)
	if err != nil {
		return Assertion{}, err
	}
	actual, ok := lookupBrowserValue(left, data, artifacts)
	if !ok {
		return Assertion{}, fmt.Errorf("unsupported browser assertion: %s", expression)
	}
	passed, actualText, expectedText, err := compareAssertion(actual, operator, right)
	if err != nil {
		return Assertion{}, err
	}
	return finalizeAssertion(expression, expectedText, actualText, passed, expectPass), nil
}

func lookupBrowserValue(key string, data BrowserData, artifacts []Artifact) (interface{}, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "final_url":
		return data.Desktop.FinalURL, true
	case "title":
		return data.Desktop.Title, true
	case "console_errors":
		return data.Desktop.ConsoleErrors, true
	case "console_warnings":
		return data.Desktop.ConsoleWarnings, true
	case "page_errors":
		return data.Desktop.PageErrors, true
	case "failed_requests":
		return data.Desktop.FailedRequests, true
	case "http_errors":
		return data.Desktop.HTTPErrors, true
	case "desktop_screenshot":
		return hasScreenshotLabel(artifacts, "desktop"), true
	case "mobile_screenshot":
		return hasScreenshotLabel(artifacts, "mobile"), true
	}
	if strings.HasPrefix(key, "desktop.") {
		return lookupBrowserValue(strings.TrimPrefix(key, "desktop."), data, artifacts)
	}
	if strings.HasPrefix(key, "mobile.") && data.Mobile != nil {
		switch strings.TrimPrefix(key, "mobile.") {
		case "final_url":
			return data.Mobile.FinalURL, true
		case "title":
			return data.Mobile.Title, true
		case "console_errors":
			return data.Mobile.ConsoleErrors, true
		case "console_warnings":
			return data.Mobile.ConsoleWarnings, true
		case "page_errors":
			return data.Mobile.PageErrors, true
		case "failed_requests":
			return data.Mobile.FailedRequests, true
		case "http_errors":
			return data.Mobile.HTTPErrors, true
		}
	}
	return nil, false
}

func evaluateIOSAssertion(expression string, data IOSData, artifacts []Artifact, expectPass bool) (Assertion, error) {
	left, operator, right, err := splitAssertion(expression)
	if err != nil {
		return Assertion{}, err
	}
	actual, ok := lookupIOSValue(left, data, artifacts)
	if !ok {
		return Assertion{}, fmt.Errorf("unsupported ios assertion: %s", expression)
	}
	passed, actualText, expectedText, err := compareAssertion(actual, operator, right)
	if err != nil {
		return Assertion{}, err
	}
	return finalizeAssertion(expression, expectedText, actualText, passed, expectPass), nil
}

func lookupIOSValue(key string, data IOSData, artifacts []Artifact) (interface{}, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "bundle_id":
		return data.BundleID, true
	case "screen":
		return data.Screen, true
	case "state":
		return data.State, true
	case "app_launch":
		return data.AppLaunch, true
	case "simulator", "simulator_name":
		return data.Simulator.Name, true
	case "runtime":
		return data.Simulator.Runtime, true
	case "launch_args":
		return strings.Join(data.LaunchArgs, " "), true
	case "launch_errors":
		return data.Issues.LaunchErrors, true
	case "crashes":
		return data.Issues.Crashes, true
	case "fatal_logs":
		return data.Issues.FatalLogs, true
	case "screenshot":
		return hasAnyScreenshot(artifacts), true
	default:
		return nil, false
	}
}

func EvaluateCurlAssertions(expressions, failingExpressions []string, data CurlData) ([]Assertion, error) {
	var assertions []Assertion
	for _, expression := range normalizedLines(expressions) {
		assertion, err := evaluateCurlAssertion(expression, data, true)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, expression := range normalizedLines(failingExpressions) {
		assertion, err := evaluateCurlAssertion(expression, data, false)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	return assertions, nil
}

func EvaluateCLIAssertions(expressions, failingExpressions []string, data CLIData, transcript string, artifacts []Artifact) ([]Assertion, error) {
	var assertions []Assertion
	for _, expression := range normalizedLines(expressions) {
		assertion, err := evaluateCLIAssertion(expression, data, transcript, artifacts, true)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, expression := range normalizedLines(failingExpressions) {
		assertion, err := evaluateCLIAssertion(expression, data, transcript, artifacts, false)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	return assertions, nil
}

func evaluateCurlAssertion(expression string, data CurlData, expectPass bool) (Assertion, error) {
	left, operator, right, err := splitAssertion(expression)
	if err != nil {
		return Assertion{}, err
	}
	actual, ok := lookupCurlValue(left, data)
	if !ok {
		return Assertion{}, fmt.Errorf("unsupported curl assertion: %s", expression)
	}
	passed, actualText, expectedText, err := compareAssertion(actual, operator, right)
	if err != nil {
		return Assertion{}, err
	}
	return finalizeAssertion(expression, expectedText, actualText, passed, expectPass), nil
}

func finalizeAssertion(expression, expectedText, actualText string, rawPassed, expectPass bool) Assertion {
	description := expression
	message := ""
	passed := rawPassed
	if !expectPass {
		description = fmt.Sprintf("NOT (%s)", expression)
		if rawPassed {
			passed = false
			message = "expected this assertion to fail, but it passed"
		} else {
			passed = true
		}
	}
	result := AssertionFail
	if passed {
		result = AssertionPass
	}
	return Assertion{
		Description: description,
		Expected:    expectedText,
		Actual:      actualText,
		Result:      result,
		Message:     message,
	}
}

func lookupCurlValue(key string, data CurlData) (interface{}, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	switch {
	case key == "status":
		return data.ResponseStatus, true
	case key == "exit_code":
		return data.ExitCode, true
	case key == "body":
		return data.Body, true
	case strings.HasPrefix(key, "header."):
		name := strings.TrimPrefix(key, "header.")
		value, ok := data.Headers[strings.ToLower(name)]
		return value, ok
	default:
		return nil, false
	}
}

func evaluateCLIAssertion(expression string, data CLIData, transcript string, artifacts []Artifact, expectPass bool) (Assertion, error) {
	left, operator, right, err := splitAssertion(expression)
	if err != nil {
		return Assertion{}, err
	}
	actual, ok := lookupCLIValue(left, data, transcript, artifacts)
	if !ok {
		return Assertion{}, fmt.Errorf("unsupported cli assertion: %s", expression)
	}
	passed, actualText, expectedText, err := compareAssertion(actual, operator, right)
	if err != nil {
		return Assertion{}, err
	}
	return finalizeAssertion(expression, expectedText, actualText, passed, expectPass), nil
}

func lookupCLIValue(key string, data CLIData, transcript string, artifacts []Artifact) (interface{}, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "output", "transcript":
		return transcript, true
	case "command":
		return data.Command, true
	case "session", "session_id":
		return data.SessionID, true
	case "tool":
		return data.Tool, true
	case "exit_code":
		if data.ExitCode == nil {
			return nil, false
		}
		return *data.ExitCode, true
	case "screenshot":
		return hasAnyArtifactKind(artifacts, ArtifactImage), true
	default:
		return nil, false
	}
}

func splitAssertion(expression string) (string, string, string, error) {
	for _, operator := range []string{" contains ", " = "} {
		if left, right, ok := strings.Cut(expression, operator); ok {
			return strings.TrimSpace(left), strings.TrimSpace(operator), strings.TrimSpace(right), nil
		}
	}
	return "", "", "", fmt.Errorf("unsupported assertion expression: %s", expression)
}

func compareAssertion(actual interface{}, operator, expected string) (bool, string, string, error) {
	operator = strings.TrimSpace(operator)
	switch value := actual.(type) {
	case int:
		want, err := strconv.Atoi(expected)
		if err != nil {
			return false, "", "", fmt.Errorf("expected integer in assertion, got %q", expected)
		}
		return value == want, strconv.Itoa(value), strconv.Itoa(want), nil
	case bool:
		want := strings.EqualFold(expected, "true")
		return value == want, strconv.FormatBool(value), strconv.FormatBool(want), nil
	case string:
		if operator == "contains" {
			return strings.Contains(value, expected), value, expected, nil
		}
		return value == expected, value, expected, nil
	default:
		return false, "", "", fmt.Errorf("unsupported assertion value type")
	}
}

func hasScreenshotLabel(artifacts []Artifact, label string) bool {
	label = strings.ToLower(label)
	for _, artifact := range artifacts {
		if artifact.Kind != ArtifactImage {
			continue
		}
		if strings.Contains(strings.ToLower(artifact.Label), label) {
			return true
		}
	}
	return false
}

func hasAnyScreenshot(artifacts []Artifact) bool {
	return hasAnyArtifactKind(artifacts, ArtifactImage)
}

func hasAnyArtifactKind(artifacts []Artifact, kind string) bool {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return true
		}
	}
	return false
}

func coveredAssertionKeys(expressions, failingExpressions []string) map[string]bool {
	covered := map[string]bool{}
	for _, group := range [][]string{expressions, failingExpressions} {
		for _, expression := range normalizedLines(group) {
			left, _, _, err := splitAssertion(expression)
			if err != nil {
				continue
			}
			covered[strings.ToLower(strings.TrimSpace(left))] = true
		}
	}
	return covered
}

func implicitBrowserAssertions(covered map[string]bool, data BrowserData) []Assertion {
	var assertions []Assertion
	appendDevice := func(prefix string) {
		for _, metric := range defaultBlockingBrowserHealthChecks {
			key := metric
			if prefix != "" {
				key = prefix + "." + metric
			}
			if prefix == "" && (covered[key] || covered["desktop."+metric]) {
				continue
			}
			if prefix != "" && covered[key] {
				continue
			}
			value, ok := lookupBrowserValue(key, data, nil)
			if !ok {
				continue
			}
			actualValue, _ := value.(int)
			result := AssertionPass
			message := ""
			if actualValue != 0 {
				result = AssertionFail
				message = "implicit zero-issues policy failed"
			}
			assertions = append(assertions, Assertion{
				Description: key + " = 0",
				Expected:    "0",
				Actual:      strconv.Itoa(actualValue),
				Result:      result,
				Message:     message,
			})
		}
	}

	appendDevice("")
	if data.Mobile != nil {
		appendDevice("mobile")
	}
	return assertions
}

func implicitIOSAssertions(covered map[string]bool, data IOSData) []Assertion {
	var assertions []Assertion
	for _, metric := range defaultBlockingIOSHealthChecks {
		if covered[metric] {
			continue
		}
		value, ok := lookupIOSValue(metric, data, nil)
		if !ok {
			continue
		}
		actualValue, _ := value.(int)
		result := AssertionPass
		message := ""
		if actualValue != 0 {
			result = AssertionFail
			message = "implicit zero-issues policy failed"
		}
		assertions = append(assertions, Assertion{
			Description: metric + " = 0",
			Expected:    "0",
			Actual:      strconv.Itoa(actualValue),
			Result:      result,
			Message:     message,
		})
	}
	return assertions
}

type desktopAuditReport struct {
	App    *desktopAuditApp    `json:"app"`
	Issues *desktopAuditIssues `json:"issues"`
}

type desktopAuditApp struct {
	Name        string `json:"name"`
	BundleID    string `json:"bundleId"`
	PID         int    `json:"pid"`
	State       string `json:"state"`
	WindowTitle string `json:"windowTitle"`
}

type desktopAuditIssues struct {
	Crashes   int `json:"crashes"`
	FatalLogs int `json:"fatalLogs"`
}

func ParseDesktopReport(path string) (DesktopData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DesktopData{}, err
	}
	var report desktopAuditReport
	if err := json.Unmarshal(data, &report); err != nil {
		return DesktopData{}, fmt.Errorf("parse desktop report: %w", err)
	}

	result := DesktopData{}
	if report.App != nil {
		result.AppName = report.App.Name
		result.BundleID = report.App.BundleID
		result.PID = report.App.PID
		result.State = report.App.State
		result.WindowTitle = report.App.WindowTitle
	}
	if report.Issues != nil {
		result.Issues = DesktopIssueSummary{
			Crashes:   report.Issues.Crashes,
			FatalLogs: report.Issues.FatalLogs,
		}
	}
	return result, nil
}

func EvaluateDesktopAssertions(expressions, failingExpressions []string, data DesktopData, artifacts []Artifact) ([]Assertion, error) {
	var assertions []Assertion
	covered := coveredAssertionKeys(expressions, failingExpressions)
	for _, expression := range normalizedLines(expressions) {
		assertion, err := evaluateDesktopAssertion(expression, data, artifacts, true)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, expression := range normalizedLines(failingExpressions) {
		assertion, err := evaluateDesktopAssertion(expression, data, artifacts, false)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertion)
	}
	for _, assertion := range implicitDesktopAssertions(covered, data) {
		assertions = append(assertions, assertion)
	}
	return assertions, nil
}

func evaluateDesktopAssertion(expression string, data DesktopData, artifacts []Artifact, expectPass bool) (Assertion, error) {
	left, operator, right, err := splitAssertion(expression)
	if err != nil {
		return Assertion{}, err
	}
	actual, ok := lookupDesktopValue(left, data, artifacts)
	if !ok {
		return Assertion{}, fmt.Errorf("unsupported desktop assertion: %s", expression)
	}
	passed, actualText, expectedText, err := compareAssertion(actual, operator, right)
	if err != nil {
		return Assertion{}, err
	}
	return finalizeAssertion(expression, expectedText, actualText, passed, expectPass), nil
}

func lookupDesktopValue(key string, data DesktopData, artifacts []Artifact) (interface{}, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "app_name":
		return data.AppName, true
	case "bundle_id":
		return data.BundleID, true
	case "state":
		return data.State, true
	case "window_title":
		return data.WindowTitle, true
	case "crashes":
		return data.Issues.Crashes, true
	case "fatal_logs":
		return data.Issues.FatalLogs, true
	case "screenshot":
		return hasAnyScreenshot(artifacts), true
	default:
		return nil, false
	}
}

func implicitDesktopAssertions(covered map[string]bool, data DesktopData) []Assertion {
	var assertions []Assertion
	for _, metric := range defaultBlockingDesktopHealthChecks {
		if covered[metric] {
			continue
		}
		value, ok := lookupDesktopValue(metric, data, nil)
		if !ok {
			continue
		}
		actualValue, _ := value.(int)
		result := AssertionPass
		message := ""
		if actualValue != 0 {
			result = AssertionFail
			message = "implicit zero-issues policy failed"
		}
		assertions = append(assertions, Assertion{
			Description: metric + " = 0",
			Expected:    "0",
			Actual:      strconv.Itoa(actualValue),
			Result:      result,
			Message:     message,
		})
	}
	return assertions
}

func ParseHTTPTranscript(text string) (int, map[string]string, string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	status := 0
	headers := map[string]string{}
	bodyLines := []string{}
	started := false
	inHeaders := false

	for _, line := range lines {
		if strings.HasPrefix(line, "HTTP/") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if code, err := strconv.Atoi(fields[1]); err == nil {
					status = code
					headers = map[string]string{}
					bodyLines = bodyLines[:0]
					started = true
					inHeaders = true
					continue
				}
			}
		}
		if !started {
			continue
		}
		if inHeaders {
			if strings.TrimSpace(line) == "" {
				inHeaders = false
				continue
			}
			name, value, ok := strings.Cut(line, ":")
			if ok {
				headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
			}
			continue
		}
		bodyLines = append(bodyLines, line)
	}

	body := strings.TrimSpace(strings.Join(bodyLines, "\n"))
	if len(body) > 4096 {
		body = body[:4096]
	}
	return status, headers, body
}

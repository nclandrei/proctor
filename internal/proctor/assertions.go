package proctor

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type agentBrowserAuditReport struct {
	Desktop *agentBrowserDevice `json:"desktop"`
	Mobile  *agentBrowserDevice `json:"mobile"`
}

type agentBrowserDevice struct {
	Title    string                 `json:"title"`
	FinalURL string                 `json:"finalUrl"`
	Issues   map[string]interface{} `json:"issues"`
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
	if !expectPass {
		passed = !passed
	}
	result := AssertionFail
	if passed {
		result = AssertionPass
	}
	return Assertion{
		Description: expression,
		Expected:    expectedText,
		Actual:      actualText,
		Result:      result,
	}, nil
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
	if !expectPass {
		passed = !passed
	}
	result := AssertionFail
	if passed {
		result = AssertionPass
	}
	return Assertion{
		Description: expression,
		Expected:    expectedText,
		Actual:      actualText,
		Result:      result,
	}, nil
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

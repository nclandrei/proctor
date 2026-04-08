package proctor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mockClaudeServer(t *testing.T, responseBody string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != AnthropicVersion {
			t.Errorf("expected anthropic-version %s, got %s", AnthropicVersion, r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprint(w, responseBody)
	}))
}

func validAnalysisJSON() string {
	return `{
  "description": "Login page with email and password fields, a blue Submit button at the bottom",
  "comparison": "The screenshot shows the expected login form with all required fields present",
  "findings": ["Email input field is visible and empty", "Password field is present with masked input", "Submit button is blue and enabled"],
  "concerns": [],
  "matches_intent": true
}`
}

func mockClaudeResponse(analysisJSON string) string {
	resp := apiResponse{
		Content: []apiResponseContent{
			{Type: "text", Text: analysisJSON},
		},
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

func mockClaudeErrorResponse(errType, errMsg string) string {
	resp := struct {
		Error *apiError `json:"error"`
	}{
		Error: &apiError{Type: errType, Message: errMsg},
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

func TestVisionClientAnalyzeScreenshot(t *testing.T) {
	server := mockClaudeServer(t, mockClaudeResponse(validAnalysisJSON()), http.StatusOK)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	// Create a test image file.
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	analysis, err := client.AnalyzeScreenshot(
		imgPath,
		"Valid login goes to dashboard",
		"happy-path",
		"about to verify the dashboard loads after login",
		"Navigated to login page",
	)
	if err != nil {
		t.Fatalf("AnalyzeScreenshot failed: %v", err)
	}

	if analysis.Description == "" {
		t.Fatal("expected description to be set")
	}
	if !strings.Contains(analysis.Description, "Login page") {
		t.Fatalf("expected description to mention login page, got: %s", analysis.Description)
	}
	if !analysis.MatchesIntent {
		t.Fatal("expected MatchesIntent to be true")
	}
	if len(analysis.Findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(analysis.Findings))
	}
	if len(analysis.Concerns) != 0 {
		t.Fatalf("expected 0 concerns, got %d", len(analysis.Concerns))
	}
	if analysis.Model != "test-model" {
		t.Fatalf("expected model test-model, got %s", analysis.Model)
	}
	if analysis.AnalyzedAt.IsZero() {
		t.Fatal("expected AnalyzedAt to be set")
	}
}

func TestVisionClientAPIError(t *testing.T) {
	server := mockClaudeServer(t,
		mockClaudeErrorResponse("invalid_request_error", "model not found"),
		http.StatusBadRequest,
	)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "bad-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := client.AnalyzeScreenshot(imgPath, "test", "happy-path", "", "")
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "Claude API error") {
		t.Fatalf("expected Claude API error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected error message to contain 'model not found', got: %v", err)
	}
}

func TestVisionClientMissingFile(t *testing.T) {
	server := mockClaudeServer(t, mockClaudeResponse(validAnalysisJSON()), http.StatusOK)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	_, err := client.AnalyzeScreenshot("/nonexistent/path.png", "test", "happy-path", "", "")
	if err == nil {
		t.Fatal("expected file not found error")
	}
	if !strings.Contains(err.Error(), "read screenshot") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestParseAnalysisResponse(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		analysis, err := parseAnalysisResponse(validAnalysisJSON())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis.Description == "" {
			t.Fatal("expected description")
		}
		if !analysis.MatchesIntent {
			t.Fatal("expected matches_intent true")
		}
	})

	t.Run("JSON with markdown fences", func(t *testing.T) {
		fenced := "```json\n" + validAnalysisJSON() + "\n```"
		analysis, err := parseAnalysisResponse(fenced)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis.Description == "" {
			t.Fatal("expected description")
		}
	})

	t.Run("missing description", func(t *testing.T) {
		_, err := parseAnalysisResponse(`{"comparison":"ok","findings":["a"],"concerns":[],"matches_intent":true}`)
		if err == nil {
			t.Fatal("expected error for missing description")
		}
		if !strings.Contains(err.Error(), "missing description") {
			t.Fatalf("expected missing description error, got: %v", err)
		}
	})

	t.Run("no findings", func(t *testing.T) {
		_, err := parseAnalysisResponse(`{"description":"test","comparison":"ok","findings":[],"concerns":[],"matches_intent":true}`)
		if err == nil {
			t.Fatal("expected error for no findings")
		}
		if !strings.Contains(err.Error(), "no findings") {
			t.Fatalf("expected no findings error, got: %v", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseAnalysisResponse("this is not json")
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "invalid JSON") {
			t.Fatalf("expected invalid JSON error, got: %v", err)
		}
	})

	t.Run("null concerns becomes empty slice", func(t *testing.T) {
		input := `{"description":"test","comparison":"ok","findings":["a"],"matches_intent":true}`
		analysis, err := parseAnalysisResponse(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis.Concerns == nil {
			t.Fatal("expected concerns to be non-nil empty slice")
		}
		if len(analysis.Concerns) != 0 {
			t.Fatalf("expected 0 concerns, got %d", len(analysis.Concerns))
		}
	})

	t.Run("does not match intent", func(t *testing.T) {
		input := `{"description":"error page","comparison":"does not match","findings":["shows 500"],"concerns":["server error"],"matches_intent":false}`
		analysis, err := parseAnalysisResponse(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis.MatchesIntent {
			t.Fatal("expected MatchesIntent to be false")
		}
		if len(analysis.Concerns) != 1 {
			t.Fatalf("expected 1 concern, got %d", len(analysis.Concerns))
		}
	})
}

func TestBuildAnalysisPrompt(t *testing.T) {
	t.Run("with all fields", func(t *testing.T) {
		prompt := buildAnalysisPrompt("Login flow", "happy-path", "about to test login", "Clicked Submit")
		if !strings.Contains(prompt, "Login flow") {
			t.Fatal("expected prompt to contain scenario label")
		}
		if !strings.Contains(prompt, "happy-path") {
			t.Fatal("expected prompt to contain scenario kind")
		}
		if !strings.Contains(prompt, "about to test login") {
			t.Fatal("expected prompt to contain pre-notes")
		}
		if !strings.Contains(prompt, "Clicked Submit") {
			t.Fatal("expected prompt to contain step action")
		}
		if !strings.Contains(prompt, "matches_intent") {
			t.Fatal("expected prompt to contain JSON format instructions")
		}
	})

	t.Run("without optional fields", func(t *testing.T) {
		prompt := buildAnalysisPrompt("Login flow", "happy-path", "", "")
		if strings.Contains(prompt, "Pre-test intent:") {
			t.Fatal("expected prompt to omit pre-test intent when empty")
		}
		if strings.Contains(prompt, "Step action:") {
			t.Fatal("expected prompt to omit step action when empty")
		}
	})
}

func TestInferMediaType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"screenshot.png", "image/png"},
		{"screenshot.PNG", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"anim.gif", "image/gif"},
		{"modern.webp", "image/webp"},
		{"unknown.bmp", "image/png"}, // defaults to png
		{"noext", "image/png"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := inferMediaType(tc.path)
			if got != tc.want {
				t.Fatalf("inferMediaType(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestNewVisionClientRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := NewVisionClient()
	if err == nil {
		t.Fatal("expected error when ANTHROPIC_API_KEY is not set")
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("expected error to mention ANTHROPIC_API_KEY, got: %v", err)
	}
}

func TestNewVisionClientDefaults(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-123")
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")

	client, err := NewVisionClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.APIKey != "test-key-123" {
		t.Fatalf("expected API key test-key-123, got %s", client.APIKey)
	}
	if client.Model != DefaultVisionModel {
		t.Fatalf("expected default model %s, got %s", DefaultVisionModel, client.Model)
	}
	if client.BaseURL != DefaultVisionBaseURL {
		t.Fatalf("expected default base URL %s, got %s", DefaultVisionBaseURL, client.BaseURL)
	}
}

func TestNewVisionClientCustomOverrides(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "custom-key")
	t.Setenv("ANTHROPIC_MODEL", "claude-opus-4-20250514")
	t.Setenv("ANTHROPIC_BASE_URL", "https://custom.api.example.com/")

	client, err := NewVisionClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.Model != "claude-opus-4-20250514" {
		t.Fatalf("expected custom model, got %s", client.Model)
	}
	if client.BaseURL != "https://custom.api.example.com" {
		t.Fatalf("expected trailing slash stripped, got %s", client.BaseURL)
	}
}

func TestAnalyzeScreenshotsIntegration(t *testing.T) {
	// Set up a mock Claude server.
	server := mockClaudeServer(t, mockClaudeResponse(validAnalysisJSON()), http.StatusOK)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	store, run, repo := setupLogFixture(t)

	// Log a step with a screenshot.
	shot := writeScreenshotFixture(t, repo, "analyze.png", "analyze-test-image")
	if _, err := LogStep(store, run, LogStepOptions{
		ScenarioID:     "happy-path",
		SessionID:      "analyze-session",
		Surface:        SurfaceBrowser,
		ScreenshotPath: shot,
		Action:         "Navigated to the login page and verified form visibility",
	}); err != nil {
		t.Fatal(err)
	}

	// Run analysis.
	results, err := AnalyzeScreenshots(store, run, client, AnalyzeOptions{
		ScenarioID: "happy-path",
	})
	if err != nil {
		t.Fatalf("AnalyzeScreenshots failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.ScenarioID != "happy-path" {
		t.Fatalf("expected scenario happy-path, got %s", result.ScenarioID)
	}
	if result.LogEntryID == "" {
		t.Fatal("expected LogEntryID to be set for log-sourced screenshot")
	}
	if result.EvidenceID != "" {
		t.Fatal("expected EvidenceID to be empty for log-sourced screenshot")
	}
	if !result.MatchesIntent {
		t.Fatal("expected MatchesIntent to be true")
	}

	// Verify analysis was stored in the ledger.
	stored, err := store.AnalysisLedger(run).LoadForScenario("happy-path")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored analysis, got %d", len(stored))
	}
	if stored[0].ID != result.ID {
		t.Fatalf("stored ID mismatch: %s != %s", stored[0].ID, result.ID)
	}
}

func TestAnalyzeScreenshotsFromEvidence(t *testing.T) {
	server := mockClaudeServer(t, mockClaudeResponse(validAnalysisJSON()), http.StatusOK)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	store, run, repo := setupLogFixture(t)

	// Record browser evidence (which creates screenshot artifacts).
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/dashboard", 0, 0, 0, 0))
	desktopShot := writeScreenshotFixture(t, repo, "desktop.png", "desktop-evidence-image")
	mobileShot := writeScreenshotFixture(t, repo, "mobile.png", "mobile-evidence-image")

	filePreNote(t, store, run, "happy-path", "evidence-session")
	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "evidence-session",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{"final_url contains /dashboard"},
	}); err != nil {
		t.Fatal(err)
	}

	// Analyze should pick up screenshots from evidence.
	results, err := AnalyzeScreenshots(store, run, client, AnalyzeOptions{
		ScenarioID: "happy-path",
	})
	if err != nil {
		t.Fatalf("AnalyzeScreenshots failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (desktop + mobile), got %d", len(results))
	}

	// All should have evidence IDs set.
	for _, r := range results {
		if r.EvidenceID == "" {
			t.Fatal("expected EvidenceID to be set for evidence-sourced screenshot")
		}
	}
}

func TestAnalyzeScreenshotsNoScreenshots(t *testing.T) {
	server := mockClaudeServer(t, mockClaudeResponse(validAnalysisJSON()), http.StatusOK)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	store, run, _ := setupLogFixture(t)

	_, err := AnalyzeScreenshots(store, run, client, AnalyzeOptions{
		ScenarioID: "happy-path",
	})
	if err == nil {
		t.Fatal("expected error when no screenshots found")
	}
	if !strings.Contains(err.Error(), "no screenshots found") {
		t.Fatalf("expected no-screenshots error, got: %v", err)
	}
}

func TestAnalyzeScreenshotsUnknownScenario(t *testing.T) {
	server := mockClaudeServer(t, mockClaudeResponse(validAnalysisJSON()), http.StatusOK)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	store, run, _ := setupLogFixture(t)

	_, err := AnalyzeScreenshots(store, run, client, AnalyzeOptions{
		ScenarioID: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for unknown scenario")
	}
	if !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("expected unknown scenario error, got: %v", err)
	}
}

func TestAnalyzeScreenshotsRequiresScenario(t *testing.T) {
	client := &VisionClient{APIKey: "test"}
	store, run, _ := setupLogFixture(t)

	_, err := AnalyzeScreenshots(store, run, client, AnalyzeOptions{})
	if err == nil {
		t.Fatal("expected error for missing scenario")
	}
	if !strings.Contains(err.Error(), "--scenario is required") {
		t.Fatalf("expected scenario required error, got: %v", err)
	}
}

func TestAnalyzeScreenshotsSessionFilter(t *testing.T) {
	server := mockClaudeServer(t, mockClaudeResponse(validAnalysisJSON()), http.StatusOK)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	store, run, repo := setupLogFixture(t)

	// Log steps in two sessions.
	shot1 := writeScreenshotFixture(t, repo, "a1.png", "session-a-image")
	if _, err := LogStep(store, run, LogStepOptions{
		ScenarioID: "happy-path", SessionID: "session-A", Surface: SurfaceBrowser,
		ScreenshotPath: shot1, Action: "Session A step with enough characters here",
	}); err != nil {
		t.Fatal(err)
	}

	shot2 := writeScreenshotFixture(t, repo, "b1.png", "session-b-image")
	if _, err := LogStep(store, run, LogStepOptions{
		ScenarioID: "happy-path", SessionID: "session-B", Surface: SurfaceBrowser,
		ScreenshotPath: shot2, Action: "Session B step with enough characters here",
	}); err != nil {
		t.Fatal(err)
	}

	// Analyze only session-A.
	results, err := AnalyzeScreenshots(store, run, client, AnalyzeOptions{
		ScenarioID: "happy-path",
		SessionID:  "session-A",
	})
	if err != nil {
		t.Fatalf("AnalyzeScreenshots failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for session-A filter, got %d", len(results))
	}
}

func TestVisionClientServerError(t *testing.T) {
	server := mockClaudeServer(t, "Internal Server Error", http.StatusInternalServerError)
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		MaxTokens:  1024,
		HTTPClient: server.Client(),
	}

	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := client.AnalyzeScreenshot(imgPath, "test", "happy-path", "", "")
	if err == nil {
		t.Fatal("expected error for server error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status 500 in error, got: %v", err)
	}
}

func TestVisionRequestStructure(t *testing.T) {
	// Verify the API request has the right structure.
	var capturedReq apiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Errorf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mockClaudeResponse(validAnalysisJSON()))
	}))
	defer server.Close()

	client := &VisionClient{
		APIKey:     "test-api-key",
		Model:      "claude-test-model",
		BaseURL:    server.URL,
		MaxTokens:  2048,
		HTTPClient: server.Client(),
	}

	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(imgPath, []byte("test-image-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := client.AnalyzeScreenshot(imgPath, "test scenario", "happy-path", "pre-note", "step action"); err != nil {
		t.Fatal(err)
	}

	if capturedReq.Model != "claude-test-model" {
		t.Fatalf("expected model claude-test-model, got %s", capturedReq.Model)
	}
	if capturedReq.MaxTokens != 2048 {
		t.Fatalf("expected max_tokens 2048, got %d", capturedReq.MaxTokens)
	}
	if len(capturedReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(capturedReq.Messages))
	}
	msg := capturedReq.Messages[0]
	if msg.Role != "user" {
		t.Fatalf("expected role user, got %s", msg.Role)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content parts (image + text), got %d", len(msg.Content))
	}
	if msg.Content[0].Type != "image" {
		t.Fatalf("expected first content part to be image, got %s", msg.Content[0].Type)
	}
	if msg.Content[0].Source == nil {
		t.Fatal("expected image source to be set")
	}
	if msg.Content[0].Source.Type != "base64" {
		t.Fatalf("expected base64 source, got %s", msg.Content[0].Source.Type)
	}
	if msg.Content[0].Source.MediaType != "image/png" {
		t.Fatalf("expected image/png, got %s", msg.Content[0].Source.MediaType)
	}
	if msg.Content[1].Type != "text" {
		t.Fatalf("expected second content part to be text, got %s", msg.Content[1].Type)
	}
	if !strings.Contains(msg.Content[1].Text, "test scenario") {
		t.Fatal("expected prompt to contain scenario label")
	}
}

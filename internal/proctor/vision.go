package proctor

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultVisionModel   = "claude-sonnet-4-20250514"
	DefaultVisionBaseURL = "https://api.anthropic.com"
	DefaultMaxTokens     = 4096
	AnthropicVersion     = "2023-06-01"
)

// VisionClient calls the Claude API to analyze screenshots.
type VisionClient struct {
	APIKey     string
	Model      string
	BaseURL    string
	MaxTokens  int
	HTTPClient *http.Client
}

// NewVisionClient creates a VisionClient from environment variables.
// ANTHROPIC_API_KEY is required. ANTHROPIC_MODEL and ANTHROPIC_BASE_URL
// are optional overrides.
func NewVisionClient() (*VisionClient, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set; required for proctor analyze")
	}
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = DefaultVisionModel
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultVisionBaseURL
	}
	return &VisionClient{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		MaxTokens:  DefaultMaxTokens,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// screenshotTarget is one screenshot to analyze with its context.
type screenshotTarget struct {
	Path       string // absolute path to the image file
	EvidenceID string // evidence ID if from record, empty if from log
	LogEntryID string // log entry ID if from log, empty if from record
	Action     string // step action description (from log entry)
}

// AnalyzeScreenshot sends a single screenshot to the Claude API for
// vision analysis in the context of a scenario.
func (c *VisionClient) AnalyzeScreenshot(imagePath, scenarioLabel, scenarioKind, preNotes, stepAction string) (*VisionAnalysis, error) {
	imageData, mediaType, err := readImageBase64(imagePath)
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}

	prompt := buildAnalysisPrompt(scenarioLabel, scenarioKind, preNotes, stepAction)

	respText, err := c.callAPI(imageData, mediaType, prompt)
	if err != nil {
		return nil, err
	}

	analysis, err := parseAnalysisResponse(respText)
	if err != nil {
		return nil, fmt.Errorf("parse analysis response: %w", err)
	}
	analysis.Model = c.Model
	analysis.AnalyzedAt = time.Now().UTC()
	return analysis, nil
}

func buildAnalysisPrompt(scenarioLabel, scenarioKind, preNotes, stepAction string) string {
	var b strings.Builder
	b.WriteString("You are analyzing a screenshot taken during manual verification of a software feature.\n\n")
	b.WriteString(fmt.Sprintf("Scenario: %s\n", scenarioLabel))
	b.WriteString(fmt.Sprintf("Scenario kind: %s\n", scenarioKind))
	if preNotes != "" {
		b.WriteString(fmt.Sprintf("Pre-test intent: %s\n", preNotes))
	}
	if stepAction != "" {
		b.WriteString(fmt.Sprintf("Step action: %s\n", stepAction))
	}
	b.WriteString(`
Look at this screenshot carefully and:
1. Describe exactly what you see on screen (UI elements, text, layout, state)
2. Compare what you see against the scenario requirements stated above
3. List specific findings (what matches expectations, what doesn't)
4. List any concerns (visual bugs, missing elements, unexpected states, errors)
5. Determine whether the screenshot matches the scenario's intent

Respond in this exact JSON format and nothing else:
{
  "description": "detailed description of what is visible on screen",
  "comparison": "how what you see compares to the scenario requirements",
  "findings": ["finding 1", "finding 2"],
  "concerns": ["concern 1"],
  "matches_intent": true
}

If there are no concerns, use an empty array. Always include at least one finding.
Respond with ONLY the JSON object, no markdown fences or extra text.`)
	return b.String()
}

// apiRequest is the Claude Messages API request body.
type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string           `json:"role"`
	Content []apiContentPart `json:"content"`
}

type apiContentPart struct {
	Type   string          `json:"type"`
	Text   string          `json:"text,omitempty"`
	Source *apiImageSource `json:"source,omitempty"`
}

type apiImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// apiResponse is a minimal parse of the Claude Messages API response.
type apiResponse struct {
	Content []apiResponseContent `json:"content"`
	Error   *apiError            `json:"error,omitempty"`
}

type apiResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (c *VisionClient) callAPI(imageBase64, mediaType, prompt string) (string, error) {
	reqBody := apiRequest{
		Model:     c.Model,
		MaxTokens: c.MaxTokens,
		Messages: []apiMessage{{
			Role: "user",
			Content: []apiContentPart{
				{
					Type: "image",
					Source: &apiImageSource{
						Type:      "base64",
						MediaType: mediaType,
						Data:      imageBase64,
					},
				},
				{
					Type: "text",
					Text: prompt,
				},
			},
		}},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal API request: %w", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create API request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", AnthropicVersion)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call Claude API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read API response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiResp apiResponse
		if json.Unmarshal(respBody, &apiResp) == nil && apiResp.Error != nil {
			return "", fmt.Errorf("Claude API error (%d): %s: %s", resp.StatusCode, apiResp.Error.Type, apiResp.Error.Message)
		}
		return "", fmt.Errorf("Claude API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}

	for _, content := range apiResp.Content {
		if content.Type == "text" {
			return content.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in API response")
}

func readImageBase64(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	mediaType := inferMediaType(path)
	return encoded, mediaType, nil
}

func inferMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

// analysisJSON is the expected JSON structure from the Claude response.
type analysisJSON struct {
	Description   string   `json:"description"`
	Comparison    string   `json:"comparison"`
	Findings      []string `json:"findings"`
	Concerns      []string `json:"concerns"`
	MatchesIntent bool     `json:"matches_intent"`
}

func parseAnalysisResponse(text string) (*VisionAnalysis, error) {
	// Strip markdown code fences if present.
	cleaned := strings.TrimSpace(text)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		// Remove first and last lines (fences).
		if len(lines) >= 3 {
			lines = lines[1 : len(lines)-1]
		}
		cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var parsed analysisJSON
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON in response: %w\nraw: %s", err, text)
	}

	if parsed.Description == "" {
		return nil, fmt.Errorf("analysis response missing description")
	}
	if len(parsed.Findings) == 0 {
		return nil, fmt.Errorf("analysis response has no findings")
	}
	if parsed.Concerns == nil {
		parsed.Concerns = []string{}
	}

	return &VisionAnalysis{
		Description:   parsed.Description,
		Comparison:    parsed.Comparison,
		Findings:      parsed.Findings,
		Concerns:      parsed.Concerns,
		MatchesIntent: parsed.MatchesIntent,
	}, nil
}

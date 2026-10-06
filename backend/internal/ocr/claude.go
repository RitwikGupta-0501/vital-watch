package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ClaudeProvider implements Provider using Anthropic Claude Messages API via net/http
type ClaudeProvider struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClaudeProvider creates a new ClaudeProvider instance
func NewClaudeProvider(apiKey string) *ClaudeProvider {
	model := strings.TrimSpace(os.Getenv("CLAUDE_MODEL"))
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &ClaudeProvider{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (c *ClaudeProvider) Name() string {
	return "claude"
}

func (c *ClaudeProvider) SupportsMIME(mimeType string) bool {
	clean := strings.ToLower(strings.TrimSpace(mimeType))
	switch clean {
	case "application/pdf", "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	default:
		return false
	}
}

func (c *ClaudeProvider) ExtractPrescription(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
	if !c.SupportsMIME(mimeType) {
		return nil, fmt.Errorf("%w: %s not supported by %s", ErrUnsupportedMIME, mimeType, c.Name())
	}

	cleanMIME := strings.ToLower(strings.TrimSpace(mimeType))
	if cleanMIME == "image/jpg" {
		cleanMIME = "image/jpeg"
	}

	b64Data := base64.StdEncoding.EncodeToString(fileBytes)

	var mediaBlock map[string]any
	if cleanMIME == "application/pdf" {
		mediaBlock = map[string]any{
			"type": "document",
			"source": map[string]string{
				"type":       "base64",
				"media_type": "application/pdf",
				"data":       b64Data,
			},
		}
	} else {
		mediaBlock = map[string]any{
			"type": "image",
			"source": map[string]string{
				"type":       "base64",
				"media_type": cleanMIME,
				"data":       b64Data,
			},
		}
	}

	reqPayload := map[string]any{
		"model":      c.model,
		"max_tokens": 2048,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []any{
					mediaBlock,
					map[string]string{
						"type": "text",
						"text": clinicalPrompt,
					},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal claude request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create claude http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if cleanMIME == "application/pdf" {
		httpReq.Header.Set("anthropic-beta", "pdfs-2024-09-25")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("claude vision request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read claude response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, fmt.Errorf("%w: claude API returned client error %d: %s", ErrPermanent, resp.StatusCode, string(respBytes))
		}
		return nil, fmt.Errorf("claude API returned error status %d: %s", resp.StatusCode, string(respBytes))
	}

	var claudeResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBytes, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to decode claude response structure: %w", err)
	}
	var rawText string
	for _, block := range claudeResp.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			rawText = strings.TrimSpace(block.Text)
			break
		}
	}
	if rawText == "" {
		return nil, fmt.Errorf("%w: claude response contained no valid text content block", ErrPermanent)
	}
	start := strings.Index(rawText, "{")
	end := strings.LastIndex(rawText, "}")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("%w: no valid JSON object detected in claude response: %s", ErrPermanent, rawText)
	}
	jsonStr := rawText[start : end+1]

	var result ExtractedPrescription
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("%w: failed to parse claude JSON extraction: %v", ErrPermanent, err)
	}

	result.Provider = c.Name()
	return &result, nil
}

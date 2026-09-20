package ocr

import (
	"bytes"
	"os"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider implements Provider using OpenAI Vision API via lightweight net/http
type OpenAIProvider struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIProvider creates a new OpenAIProvider instance
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIProvider{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (o *OpenAIProvider) Name() string {
	return "openai"
}

// SupportsMIME explicitly rejects application/pdf because OpenAI Chat Completions rejects PDFs
func (o *OpenAIProvider) SupportsMIME(mimeType string) bool {
	clean := strings.ToLower(strings.TrimSpace(mimeType))
	switch clean {
	case "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	default:
		return false
	}
}

func (o *OpenAIProvider) ExtractPrescription(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
	if !o.SupportsMIME(mimeType) {
		return nil, fmt.Errorf("%w: %s not supported by %s (PDFs unsupported)", ErrUnsupportedMIME, mimeType, o.Name())
	}

	cleanMIME := strings.ToLower(strings.TrimSpace(mimeType))
	if cleanMIME == "image/jpg" {
		cleanMIME = "image/jpeg"
	}

	b64Data := base64.StdEncoding.EncodeToString(fileBytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", cleanMIME, b64Data)

	reqPayload := map[string]any{
		"model": o.model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": clinicalPrompt},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": dataURL,
						},
					},
				},
			},
		},
		"response_format": map[string]string{"type": "json_object"},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create openai http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai vision request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read openai response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, fmt.Errorf("%w: openai API returned client error %d: %s", ErrPermanent, resp.StatusCode, string(respBytes))
		}
		return nil, fmt.Errorf("openai API returned error status %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai response structure: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned empty choices")
	}

	rawText := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	start := strings.Index(rawText, "{")
	end := strings.LastIndex(rawText, "}")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("%w: no valid JSON object detected in openai response: %s", ErrPermanent, rawText)
	}
	jsonStr := rawText[start : end+1]

	var result ExtractedPrescription
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("%w: failed to parse openai JSON extraction: %v", ErrPermanent, err)
	}

	result.Provider = o.Name()
	return &result, nil
}

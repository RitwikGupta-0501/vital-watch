package ocr

import (
	"context"
	"os"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

const clinicalPrompt = `You are an expert clinical pharmacologist and medical OCR assistant.
Analyze this medical prescription slip (handwritten or typed) and extract all prescribed medications.

Respond strictly with a JSON object matching this schema:
{
  "medications": [
    {
      "medication_name": "string (name of the drug, e.g., Amoxicillin)",
      "dosage": "string (e.g., 500mg, 10ml, or empty)",
      "frequency": "string (e.g., TID, Twice daily, Every 8 hours, or empty)",
      "duration": "string (e.g., 7 days, 1 month, or empty)",
      "timing": "string (e.g., After meals, Before bedtime, or empty)",
      "instructions": "string (e.g., Take with a full glass of water, or empty)"
    }
  ],
  "confidence": 0.95
}

If no medications can be detected or the image is illegible, return {"medications": [], "confidence": 0.0}.
Do NOT wrap the JSON in backticks or markdown fences.`

// GeminiProvider implements Provider using Google Gemini Vision API
type GeminiProvider struct {
	client *genai.Client
	model  string
}

// NewGeminiProvider creates a new GeminiProvider instance
func NewGeminiProvider(ctx context.Context, apiKey string) (*GeminiProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini api key is required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}
	model := strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiProvider{
		client: client,
		model:  model,
	}, nil
}

func (g *GeminiProvider) Name() string {
	return "gemini"
}

func (g *GeminiProvider) SupportsMIME(mimeType string) bool {
	clean := strings.ToLower(strings.TrimSpace(mimeType))
	switch clean {
	case "application/pdf", "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	default:
		return false
	}
}

func (g *GeminiProvider) ExtractPrescription(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
	if !g.SupportsMIME(mimeType) {
		return nil, fmt.Errorf("%w: %s not supported by %s", ErrUnsupportedMIME, mimeType, g.Name())
	}

	cleanMIME := strings.ToLower(strings.TrimSpace(mimeType))
	if cleanMIME == "image/jpg" {
		cleanMIME = "image/jpeg"
	}

	content := &genai.Content{
		Parts: []*genai.Part{
			genai.NewPartFromBytes(fileBytes, cleanMIME),
			genai.NewPartFromText(clinicalPrompt),
		},
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, []*genai.Content{content}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("gemini vision extraction failed: %w", err)
	}

	rawText := strings.TrimSpace(resp.Text())
	if rawText == "" {
		return nil, fmt.Errorf("%w: gemini vision returned empty response (generation blocked or no candidate parts)", ErrPermanent)
	}
	start := strings.Index(rawText, "{")
	end := strings.LastIndex(rawText, "}")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("%w: no valid JSON object detected in gemini response: %s", ErrPermanent, rawText)
	}
	jsonStr := rawText[start : end+1]

	var result ExtractedPrescription
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("%w: failed to parse gemini JSON response (%s): %v", ErrPermanent, jsonStr, err)
	}

	result.Provider = g.Name()
	return &result, nil
}

package ocr

import (
	"context"
	"log"
	"os"
	"strings"
)

// Manager manages OCR providers, their priority order, and fallback orchestration
type Manager struct {
	chain   *FallbackChain
	enabled bool
}

// NewManagerFromEnv initializes the OCR manager from system environment variables
func NewManagerFromEnv(ctx context.Context) *Manager {
	ocrEnabledStr := strings.ToLower(strings.TrimSpace(os.Getenv("OCR_ENABLED")))
	if ocrEnabledStr == "false" || ocrEnabledStr == "0" {
		log.Println("[OCR Manager] OCR processing explicitly disabled via OCR_ENABLED=false")
		return &Manager{enabled: false}
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		geminiKey = os.Getenv("GOOGLE_API_KEY")
	}
	openaiKey := os.Getenv("OPENAI_API_KEY")
	claudeKey := os.Getenv("ANTHROPIC_API_KEY")
	if claudeKey == "" {
		claudeKey = os.Getenv("CLAUDE_API_KEY")
	}

	// Read provider preference order
	providersEnv := os.Getenv("OCR_PROVIDERS")
	if providersEnv == "" {
		providersEnv = "gemini,claude,openai"
	}
	preferredList := strings.Split(providersEnv, ",")

	var activeProviders []Provider
	for _, name := range preferredList {
		name = strings.ToLower(strings.TrimSpace(name))
		switch name {
		case "gemini":
			if geminiKey != "" {
				gp, err := NewGeminiProvider(ctx, geminiKey)
				if err != nil {
					log.Printf("[OCR Manager] Failed to initialize Gemini provider: %v", err)
				} else {
					activeProviders = append(activeProviders, gp)
					log.Println("[OCR Manager] Registered Gemini Vision Provider (Primary/Active)")
				}
			}
		case "claude", "anthropic":
			if claudeKey != "" {
				cp := NewClaudeProvider(claudeKey)
				activeProviders = append(activeProviders, cp)
				log.Println("[OCR Manager] Registered Anthropic Claude Vision Provider (Active)")
			}
		case "openai":
			if openaiKey != "" {
				op := NewOpenAIProvider(openaiKey)
				activeProviders = append(activeProviders, op)
				log.Println("[OCR Manager] Registered OpenAI Vision Provider (Active)")
			}
		}
	}

	if len(activeProviders) == 0 {
		log.Println("[OCR Manager] No OCR provider API keys configured. OCR is disabled. Uploads will transition directly to needs_review for manual clinician entry.")
		return &Manager{enabled: false}
	}

	return &Manager{
		chain:   NewFallbackChain(activeProviders...),
		enabled: true,
	}
}

// NewManagerWithChain creates a manager with a pre-configured fallback chain (useful for testing)
func NewManagerWithChain(chain *FallbackChain) *Manager {
	if chain == nil || len(chain.Providers()) == 0 {
		return &Manager{enabled: false}
	}
	return &Manager{chain: chain, enabled: true}
}

func (m *Manager) IsEnabled() bool {
	return m != nil && m.enabled && m.chain != nil && len(m.chain.Providers()) > 0
}

func (m *Manager) ExtractPrescription(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
	if !m.IsEnabled() {
		return nil, ErrOCRDisabled
	}
	return m.chain.ExtractPrescription(ctx, fileBytes, mimeType)
}

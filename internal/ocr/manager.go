package ocr

import (
	"context"
	"log/slog"
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
		slog.Info("OCR processing explicitly disabled via OCR_ENABLED=false")
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
					slog.Error("Failed to initialize Gemini provider", "error", err)
				} else {
					activeProviders = append(activeProviders, gp)
					slog.Info("Registered Gemini Vision Provider", "role", "Primary/Active")
				}
			}
		case "claude", "anthropic":
			if claudeKey != "" {
				cp := NewClaudeProvider(claudeKey)
				activeProviders = append(activeProviders, cp)
				slog.Info("Registered Anthropic Claude Vision Provider", "role", "Active")
			}
		case "openai":
			if openaiKey != "" {
				op := NewOpenAIProvider(openaiKey)
				activeProviders = append(activeProviders, op)
				slog.Info("Registered OpenAI Vision Provider", "role", "Active")
			}
		}
	}

	if len(activeProviders) == 0 {
		slog.Warn("No OCR provider API keys configured. OCR is disabled. Uploads will transition directly to needs_review")
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

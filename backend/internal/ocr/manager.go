package ocr

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/RitwikGupta-0501/vital-watch/internal/crypto"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository/dbgen"
)

// Manager manages OCR providers dynamically based on tenant configuration
type Manager struct {
	repo          repository.Repository
	cipherService *crypto.CipherService
	enabled       bool
	chain         *FallbackChain
	cacheMutex    sync.RWMutex
	providerCache map[uuid.UUID]Provider
	initGroup     singleflight.Group
}

// NewManager initializes the OCR manager
func NewManager(repo repository.Repository, cipherService *crypto.CipherService) *Manager {
	ocrEnabledStr := strings.ToLower(strings.TrimSpace(os.Getenv("OCR_ENABLED")))
	if ocrEnabledStr == "false" || ocrEnabledStr == "0" {
		slog.Info("OCR processing explicitly disabled via OCR_ENABLED=false")
		return &Manager{repo: repo, cipherService: cipherService, enabled: false}
	}
	return &Manager{
		repo:          repo,
		cipherService: cipherService,
		enabled:       true,
		providerCache: make(map[uuid.UUID]Provider),
	}
}

func (m *Manager) IsEnabled() bool {
	return m != nil && m.enabled
}

func (m *Manager) ExtractForTenant(ctx context.Context, tenantID uuid.UUID, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
	if !m.IsEnabled() {
		return nil, ErrOCRDisabled
	}

	// Override for testing
	if m.chain != nil {
		return m.chain.ExtractPrescription(ctx, fileBytes, mimeType)
	}

	settings, err := m.repo.GetTenantSettings(ctx, tenantID)
	if err != nil {
		slog.WarnContext(ctx, "No OCR settings found for tenant", "tenant_id", tenantID)
		return nil, ErrOCRDisabled
	}

	if len(settings.OcrFallbackChain) == 0 {
		return nil, ErrOCRDisabled
	}

	configs, err := m.repo.GetOCRProviderConfigsByIDs(ctx, dbgen.GetOCRProviderConfigsByIDsParams{
		Column1:  settings.OcrFallbackChain,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, ErrOCRDisabled
	}



	// Quick mapping depending on sqlc generation structures (we will assume it has these fields)
	// Actually let's just map the raw struct
	type ProviderConfig struct {
		ProviderName string
		EncryptedKey []byte
		Nonce        []byte
	}
	cmap := make(map[uuid.UUID]ProviderConfig)
	for _, cfg := range configs {
		cmap[cfg.ID] = ProviderConfig{
			ProviderName: cfg.ProviderName,
			EncryptedKey: cfg.EncryptedKey,
			Nonce:        cfg.Nonce,
		}
	}

	var activeProviders []Provider

	for _, configID := range settings.OcrFallbackChain {
		cfg, ok := cmap[configID]
		if !ok {
			continue
		}

		m.cacheMutex.RLock()
		cachedProv, exists := m.providerCache[configID]
		m.cacheMutex.RUnlock()

		if exists {
			activeProviders = append(activeProviders, cachedProv)
			continue
		}

		// Use singleflight to prevent Thundering Herd when initializing providers concurrently
		val, err, _ := m.initGroup.Do(configID.String(), func() (interface{}, error) {
			// Double check if another goroutine initialized it while we were waiting
			m.cacheMutex.RLock()
			if prov, ok := m.providerCache[configID]; ok {
				m.cacheMutex.RUnlock()
				return prov, nil
			}
			m.cacheMutex.RUnlock()

			aad := append(tenantID[:], []byte(cfg.ProviderName)...)
			plaintextKey, decErr := m.cipherService.Decrypt(cfg.EncryptedKey, cfg.Nonce, aad)
			if decErr != nil {
				return nil, fmt.Errorf("failed to decrypt API key for provider %s: %w", cfg.ProviderName, decErr)
			}

			var newProv Provider
			switch strings.ToLower(cfg.ProviderName) {
			case "gemini":
				gp, initErr := NewGeminiProvider(ctx, plaintextKey)
				if initErr != nil {
					return nil, fmt.Errorf("failed to initialize Gemini Provider: %w", initErr)
				}
				newProv = gp
			case "claude", "anthropic":
				newProv = NewClaudeProvider(plaintextKey)
			case "openai":
				newProv = NewOpenAIProvider(plaintextKey)
			default:
				return nil, fmt.Errorf("unknown OCR provider %s", cfg.ProviderName)
			}

			m.cacheMutex.Lock()
			m.providerCache[configID] = newProv
			m.cacheMutex.Unlock()

			return newProv, nil
		})

		if err != nil {
			slog.ErrorContext(ctx, "Provider initialization failed", "provider", cfg.ProviderName, "error", err)
			continue
		}

		activeProviders = append(activeProviders, val.(Provider))
	}

	if len(activeProviders) == 0 {
		return nil, ErrAllProvidersFailed
	}

	chain := NewFallbackChain(activeProviders...)
	return chain.ExtractPrescription(ctx, fileBytes, mimeType)
}

// NewManagerWithChain creates a manager with a pre-configured fallback chain (useful for testing)
func NewManagerWithChain(chain *FallbackChain) *Manager {
	if chain == nil || len(chain.Providers()) == 0 {
		return &Manager{enabled: false}
	}
	return &Manager{chain: chain, enabled: true}
}

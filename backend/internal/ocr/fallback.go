package ocr

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// FallbackChain decorates a slice of Providers, executing them in priority order
// and skipping providers that do not support the target MIME type
type FallbackChain struct {
	providers []Provider
}

// NewFallbackChain creates a new FallbackChain
func NewFallbackChain(providers ...Provider) *FallbackChain {
	return &FallbackChain{providers: providers}
}

func (fc *FallbackChain) Providers() []Provider {
	return fc.providers
}

func (fc *FallbackChain) ExtractPrescription(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
	if len(fc.providers) == 0 {
		return nil, ErrAllProvidersFailed
	}

	var (
		lastErr      error
		anySupported bool
	)
	for _, p := range fc.providers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !p.SupportsMIME(mimeType) {
			slog.InfoContext(ctx, "Skipping OCR provider; MIME type unsupported", "provider", p.Name(), "mime_type", mimeType)
			continue
		}
		anySupported = true

		providerCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		res, err := p.ExtractPrescription(providerCtx, fileBytes, mimeType)
		cancel()

		if err == nil && res != nil {
			slog.InfoContext(ctx, "Extraction succeeded via OCR provider", "provider", p.Name(), "medications_count", len(res.Medications))
			res.Provider = p.Name()
			return res, nil
		}

		lastErr = err
		slog.WarnContext(ctx, "OCR provider failed; attempting next provider", "provider", p.Name(), "error", err)
	}

	if !anySupported {
		return nil, fmt.Errorf("%w: MIME type %q is not supported by any active provider", ErrUnsupportedMIME, mimeType)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrAllProvidersFailed, lastErr)
	}
	return nil, ErrAllProvidersFailed
}

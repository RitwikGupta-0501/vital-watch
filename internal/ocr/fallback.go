package ocr

import (
	"context"
	"fmt"
	"log"
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
			log.Printf("[OCR FallbackChain] Skipping provider %q: MIME type %q is not supported", p.Name(), mimeType)
			continue
		}
		anySupported = true

		res, err := p.ExtractPrescription(ctx, fileBytes, mimeType)
		if err == nil && res != nil {
			log.Printf("[OCR FallbackChain] Extraction succeeded via provider %q (found %d medications)", p.Name(), len(res.Medications))
			res.Provider = p.Name()
			return res, nil
		}

		lastErr = err
		log.Printf("[OCR FallbackChain] Provider %q failed: %v. Attempting next provider...", p.Name(), err)
	}

	if !anySupported {
		return nil, fmt.Errorf("%w: MIME type %q is not supported by any active provider", ErrUnsupportedMIME, mimeType)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrAllProvidersFailed, lastErr)
	}
	return nil, ErrAllProvidersFailed
}

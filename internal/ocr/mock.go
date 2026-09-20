package ocr

import (
	"context"
	"fmt"
	"strings"
)

// MockProvider is a test double strictly for unit testing
type MockProvider struct {
	ProviderName string
	SupportedMIMEs map[string]bool
	ExtractFunc  func(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error)
}

func NewMockProvider(name string, supportedMIMEs ...string) *MockProvider {
	m := &MockProvider{
		ProviderName:   name,
		SupportedMIMEs: make(map[string]bool),
	}
	for _, mime := range supportedMIMEs {
		m.SupportedMIMEs[strings.ToLower(strings.TrimSpace(mime))] = true
	}
	return m
}

func (m *MockProvider) Name() string {
	if m.ProviderName == "" {
		return "mock"
	}
	return m.ProviderName
}

func (m *MockProvider) SupportsMIME(mimeType string) bool {
	if len(m.SupportedMIMEs) == 0 {
		return true
	}
	return m.SupportedMIMEs[strings.ToLower(strings.TrimSpace(mimeType))]
}

func (m *MockProvider) ExtractPrescription(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
	if !m.SupportsMIME(mimeType) {
		return nil, fmt.Errorf("%w: %s not supported by %s", ErrUnsupportedMIME, mimeType, m.Name())
	}
	if m.ExtractFunc != nil {
		return m.ExtractFunc(ctx, fileBytes, mimeType)
	}
	return &ExtractedPrescription{
		Medications: []ExtractedMedication{
			{
				MedicationName: "Amoxicillin",
				Dosage:         "500mg",
				Frequency:      "TID",
				Duration:       "7 days",
				Timing:         "After meals",
				Instructions:   "Take with water",
			},
		},
		Confidence: 0.98,
		Provider:   m.Name(),
	}, nil
}

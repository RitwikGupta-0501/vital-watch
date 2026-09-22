package ocr

import (
	"context"
	"errors"
	"testing"
)

func TestFallbackChain_Failover(t *testing.T) {
	ctx := context.Background()

	// Provider 1 fails with 429
	p1 := NewMockProvider("p1")
	p1.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
		return nil, errors.New("rate limited (429)")
	}

	// Provider 2 succeeds
	p2 := NewMockProvider("p2")
	p2.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
		return &ExtractedPrescription{
			Medications: []ExtractedMedication{
				{MedicationName: "Paracetamol", Dosage: "650mg"},
			},
			Confidence: 0.99,
		}, nil
	}

	chain := NewFallbackChain(p1, p2)
	res, err := chain.ExtractPrescription(ctx, []byte("fake-image"), "image/png")
	if err != nil {
		t.Fatalf("expected failover to succeed, got error: %v", err)
	}
	if res.Provider != "p2" {
		t.Errorf("expected provider p2 to succeed, got: %s", res.Provider)
	}
	if len(res.Medications) != 1 || res.Medications[0].MedicationName != "Paracetamol" {
		t.Errorf("unexpected extracted medications: %v", res.Medications)
	}
}

func TestFallbackChain_MIMESkipping(t *testing.T) {
	ctx := context.Background()

	// Provider 1 only supports image/png (like OpenAI rejecting PDFs)
	p1 := NewMockProvider("image_only", "image/png", "image/jpeg")
	p1Called := false
	p1.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
		p1Called = true
		return nil, errors.New("should not be called for PDF")
	}

	// Provider 2 supports application/pdf (like Gemini/Claude)
	p2 := NewMockProvider("pdf_supported", "application/pdf")
	p2.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
		return &ExtractedPrescription{
			Medications: []ExtractedMedication{
				{MedicationName: "Omeprazole", Dosage: "20mg"},
			},
			Confidence: 0.95,
		}, nil
	}

	chain := NewFallbackChain(p1, p2)
	res, err := chain.ExtractPrescription(ctx, []byte("%PDF-1.4..."), "application/pdf")
	if err != nil {
		t.Fatalf("expected successful PDF extraction, got: %v", err)
	}
	if p1Called {
		t.Errorf("expected provider 1 to be skipped due to unsupported MIME, but it was called")
	}
	if res.Provider != "pdf_supported" {
		t.Errorf("expected pdf_supported provider, got: %s", res.Provider)
	}
}

func TestFallbackChain_AllFail(t *testing.T) {
	ctx := context.Background()

	p1 := NewMockProvider("p1")
	p1.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error) {
		return nil, errors.New("fail 1")
	}

	chain := NewFallbackChain(p1)
	_, err := chain.ExtractPrescription(ctx, []byte("data"), "image/jpeg")
	if err == nil || !errors.Is(err, ErrAllProvidersFailed) {
		t.Fatalf("expected ErrAllProvidersFailed, got: %v", err)
	}
}

func TestManager_DisabledState(t *testing.T) {
	ctx := context.Background()

	manager := NewManagerWithChain(nil)
	if manager.IsEnabled() {
		t.Fatalf("expected manager to be disabled when chain is nil")
	}

	_, err := manager.ExtractPrescription(ctx, []byte("data"), "image/jpeg")
	if err != ErrOCRDisabled {
		t.Fatalf("expected ErrOCRDisabled when manager is disabled, got: %v", err)
	}
}

func TestFallbackChain_AllUnsupportedMIMEReturnsErrUnsupportedMIME(t *testing.T) {
	ctx := context.Background()
	p1 := NewMockProvider("image_only", "image/png", "image/jpeg")
	chain := NewFallbackChain(p1)

	_, err := chain.ExtractPrescription(ctx, []byte("%PDF-1.4..."), "application/pdf")
	if err == nil {
		t.Fatalf("expected error for unsupported MIME, got nil")
	}
	if !errors.Is(err, ErrUnsupportedMIME) {
		t.Fatalf("expected ErrUnsupportedMIME, got: %v", err)
	}
}

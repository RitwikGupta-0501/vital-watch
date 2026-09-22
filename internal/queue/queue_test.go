package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/ocr"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func TestPrescriptionOCRWorker_DisabledOCR(t *testing.T) {
	ctx := context.Background()
	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	ocrManager := ocr.NewManagerWithChain(nil)

	updated := false
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		updated = true
		if status != "needs_review" {
			t.Errorf("expected status needs_review, got: %s", status)
		}
		return nil
	}

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: uuid.New(),
			StorageKey:     "test-key.pdf",
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error when OCR disabled, got: %v", err)
	}
	if !updated {
		t.Fatalf("expected prescription to be updated to needs_review")
	}
}

func TestPrescriptionOCRWorker_Success(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.Files[storageKey] = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	mockOCR := ocr.NewMockProvider("mock-gemini")
	mockOCR.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ocr.ExtractedPrescription, error) {
		return &ocr.ExtractedPrescription{
			Medications: []ocr.ExtractedMedication{
				{MedicationName: "Amoxicillin", Dosage: "500mg", Frequency: "TID"},
				{MedicationName: "Paracetamol", Dosage: "650mg", Frequency: "PRN"},
			},
			Confidence: 0.98,
			Provider:   "mock-gemini",
		}, nil
	}
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))

	var savedItems []models.PrescriptionItem
	var savedStatus, savedProvider string
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		savedStatus = status
		savedProvider = ocrProvider
		savedItems = items
		return nil
	}

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error on success, got: %v", err)
	}
	if savedStatus != "needs_review" {
		t.Errorf("expected status needs_review, got: %s", savedStatus)
	}
	if savedProvider != "mock-gemini" {
		t.Errorf("expected provider mock-gemini, got: %s", savedProvider)
	}
	if len(savedItems) != 2 {
		t.Fatalf("expected 2 extracted items, got %d", len(savedItems))
	}
}

func TestPrescriptionOCRWorker_PermanentFailure(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.Files[storageKey] = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	mockOCR := ocr.NewMockProvider("mock-failing")
	mockOCR.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ocr.ExtractedPrescription, error) {
		return nil, ocr.ErrPermanent
	}
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))

	updated := false
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		updated = true
		if status != "needs_review" {
			t.Errorf("expected status needs_review, got: %s", status)
		}
		return nil
	}

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1},
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error on permanent failure, got: %v", err)
	}
	if !updated {
		t.Fatalf("expected prescription to be marked needs_review")
	}
}

func TestPrescriptionOCRWorker_TransientFailure(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.Files[storageKey] = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	mockOCR := ocr.NewMockProvider("mock-transient")
	transientErr := errors.New("temporary 503 service unavailable")
	mockOCR.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ocr.ExtractedPrescription, error) {
		return nil, transientErr
	}
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1},
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Fatalf("expected transient error to be returned to River for retry, got nil")
	}
}

func TestPrescriptionOCRWorker_MaxAttemptsExceeded(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.Files[storageKey] = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	mockOCR := ocr.NewMockProvider("mock-transient")
	mockOCR.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ocr.ExtractedPrescription, error) {
		return nil, errors.New("persistent timeout")
	}
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))

	updated := false
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		updated = true
		if status != "needs_review" {
			t.Errorf("expected status needs_review, got: %s", status)
		}
		return nil
	}

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3},
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error after max attempts exceeded, got: %v", err)
	}
	if !updated {
		t.Fatalf("expected prescription to be marked needs_review after max attempts")
	}
}

func TestPrescriptionOCRWorker_AlreadyHandledStatus(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.Files[storageKey] = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	mockOCR := ocr.NewMockProvider("mock-gemini")
	mockOCR.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ocr.ExtractedPrescription, error) {
		return &ocr.ExtractedPrescription{
			Medications: []ocr.ExtractedMedication{
				{MedicationName: "Amoxicillin", Dosage: "500mg"},
			},
			Confidence: 0.95,
		}, nil
	}
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))

	// Simulate that the row was already verified or not pending_ocr (0 rows updated in DB, returns nil)
	updateCalled := false
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		updateCalled = true
		// DB repository returns nil when rowsAffected == 0
		return nil
	}

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error when prescription is already handled, got: %v", err)
	}
	if !updateCalled {
		t.Fatalf("expected UpdatePrescriptionOCRResults to be called")
	}
}

func TestPrescriptionOCRWorker_TransientStorageFailure(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	transientErr := errors.New("temporary s3 connection reset by peer")
	mockStorage.GetFileBytesFunc = func(ctx context.Context, key string) ([]byte, string, error) {
		return nil, "", transientErr
	}

	mockOCR := ocr.NewMockProvider("mock-gemini")
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))
	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Fatalf("expected transient storage error to be returned to River for retry, got nil")
	}
	if !errors.Is(err, transientErr) {
		t.Fatalf("expected transientErr, got: %v", err)
	}
}

func TestPrescriptionOCRWorker_PermanentStorageFailure(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.GetFileBytesFunc = func(ctx context.Context, key string) ([]byte, string, error) {
		return nil, "", storage.ErrFileTooLarge
	}

	var savedStatus, savedNotes string
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		savedStatus = status
		savedNotes = notes
		return nil
	}

	mockOCR := ocr.NewMockProvider("mock-gemini")
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))
	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error on permanent storage failure, got: %v", err)
	}
	if savedStatus != "needs_review" {
		t.Errorf("expected status needs_review, got: %s", savedStatus)
	}
	if savedNotes == "" {
		t.Errorf("expected savedNotes to describe storage failure, got empty")
	}
	if savedNotes == "" {
		t.Errorf("expected notes to contain error message, got empty")
	}
}

func TestPrescriptionOCRWorker_DBTransitionFailure(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.GetFileBytesFunc = func(ctx context.Context, key string) ([]byte, string, error) {
		return nil, "", storage.ErrFileTooLarge
	}

	dbErr := errors.New("database connection pool exhausted")
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		return dbErr
	}

	mockOCR := ocr.NewMockProvider("mock-gemini")
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))
	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Fatalf("expected DB transition error to be returned to River, got nil")
	}
}

func TestPrescriptionOCRWorker_StorageMaxAttempts(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-file.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	storageNetworkErr := errors.New("i/o timeout reaching S3 endpoint")
	mockStorage.GetFileBytesFunc = func(ctx context.Context, key string) ([]byte, string, error) {
		return nil, "", storageNetworkErr
	}

	var savedStatus, savedNotes string
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		savedStatus = status
		savedNotes = notes
		return nil
	}

	mockOCR := ocr.NewMockProvider("mock-gemini")
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))
	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)

	// Attempt 1: Should return error to River for retry
	jobAttempt1 := &river.Job[PrescriptionOCRArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1},
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}
	err1 := worker.Work(ctx, jobAttempt1)
	if err1 == nil {
		t.Fatalf("expected transient storage error to return error on attempt 1, got nil")
	}

	// Attempt 3: Should mark needs_review and return nil
	jobAttempt3 := &river.Job[PrescriptionOCRArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3},
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}
	err3 := worker.Work(ctx, jobAttempt3)
	if err3 != nil {
		t.Fatalf("expected nil on max attempts reached for storage, got: %v", err3)
	}
	if savedStatus != "needs_review" {
		t.Errorf("expected status needs_review, got: %s", savedStatus)
	}
	if savedNotes == "" {
		t.Errorf("expected savedNotes to describe storage failure, got empty")
	}
	if savedNotes == "" {
		t.Errorf("expected savedNotes to describe storage failure, got empty")
	}
}

func TestPrescriptionOCRWorker_StringClamping(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	storageKey := "rx-long.png"

	mockRepo := &repository.MockRepository{}
	mockStorage := storage.NewMockProvider()
	mockStorage.Files[storageKey] = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	longName := strings.Repeat("A", 300)
	longDosage := strings.Repeat("B", 150)
	longFreq := strings.Repeat("C", 150)
	longDur := strings.Repeat("D", 150)
	longTiming := strings.Repeat("E", 150)

	mockOCR := ocr.NewMockProvider("mock-provider-with-very-long-name-exceeding-fifty-chars-limit")
	mockOCR.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ocr.ExtractedPrescription, error) {
		return &ocr.ExtractedPrescription{
			Medications: []ocr.ExtractedMedication{
				{
					MedicationName: longName,
					Dosage:         longDosage,
					Frequency:      longFreq,
					Duration:       longDur,
					Timing:         longTiming,
					Instructions:   "Take with water",
				},
			},
			Confidence: 0.95,
			Provider:   mockOCR.Name(),
		}, nil
	}
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))

	var savedItems []models.PrescriptionItem
	var savedProvider string
	mockRepo.UpdatePrescriptionOCRResultsFunc = func(ctx context.Context, prescriptionID uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
		savedProvider = ocrProvider
		savedItems = items
		return nil
	}

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(savedItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(savedItems))
	}
	item := savedItems[0]
	if len(item.MedicationName) > 255 {
		t.Errorf("MedicationName length %d exceeds 255", len(item.MedicationName))
	}
	if len(item.Dosage) > 100 {
		t.Errorf("Dosage length %d exceeds 100", len(item.Dosage))
	}
	if len(item.Frequency) > 100 {
		t.Errorf("Frequency length %d exceeds 100", len(item.Frequency))
	}
	if len(item.Duration) > 100 {
		t.Errorf("Duration length %d exceeds 100", len(item.Duration))
	}
	if len(item.Timing) > 100 {
		t.Errorf("Timing length %d exceeds 100", len(item.Timing))
	}
	if len(savedProvider) > 50 {
		t.Errorf("Provider length %d exceeds 50", len(savedProvider))
	}
}

func TestPrescriptionOCRWorker_NotificationOnPermanentFailure(t *testing.T) {
	ctx := context.Background()
	prescID := uuid.New()
	doctorID := uuid.New()
	storageKey := "rx-file.png"

	notifier := notifications.NewSSEBroker()
	defer notifier.Shutdown()

	eventCh, unsub := notifier.Subscribe(doctorID)
	defer unsub()

	mockRepo := &repository.MockRepository{
		GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
			return models.Prescription{ID: id, DoctorID: doctorID}, nil
		},
		UpdatePrescriptionOCRResultsFunc: func(ctx context.Context, id uuid.UUID, status, notes, ocrProvider string, items []models.PrescriptionItem) error {
			return nil
		},
	}
	mockStorage := storage.NewMockProvider()
	mockStorage.Files[storageKey] = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	mockOCR := ocr.NewMockProvider("mock-failing")
	mockOCR.ExtractFunc = func(ctx context.Context, fileBytes []byte, mimeType string) (*ocr.ExtractedPrescription, error) {
		return nil, ocr.ErrPermanent
	}
	ocrManager := ocr.NewManagerWithChain(ocr.NewFallbackChain(mockOCR))

	worker := NewPrescriptionOCRWorker(mockRepo, mockStorage, ocrManager)
	worker.SetNotifier(notifier)

	job := &river.Job[PrescriptionOCRArgs]{
		Args: PrescriptionOCRArgs{
			PrescriptionID: prescID,
			StorageKey:     storageKey,
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Fatalf("expected nil error on permanent failure, got: %v", err)
	}

	select {
	case evt := <-eventCh:
		if evt.Type != notifications.EventOCRCompleted {
			t.Errorf("expected EventOCRCompleted, got %v", evt.Type)
		}
		if evt.DoctorID != doctorID {
			t.Errorf("expected DoctorID %v, got %v", doctorID, evt.DoctorID)
		}
		if !strings.Contains(evt.Message, "manual review required") {
			t.Errorf("expected message mentioning manual review, got: %s", evt.Message)
		}
	default:
		t.Fatalf("expected notification to be published on permanent failure")
	}
}

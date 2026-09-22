package queue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/ocr"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

type PrescriptionOCRArgs = models.PrescriptionOCRArgs

type PrescriptionOCRWorker struct {
	river.WorkerDefaults[PrescriptionOCRArgs]
	repo       repository.Repository
	storage    storage.Provider
	ocrManager *ocr.Manager
	notifier   notifications.Broker
}

func NewPrescriptionOCRWorker(repo repository.Repository, storage storage.Provider, ocrManager *ocr.Manager) *PrescriptionOCRWorker {
	return &PrescriptionOCRWorker{
		repo:       repo,
		storage:    storage,
		ocrManager: ocrManager,
	}
}

func (w *PrescriptionOCRWorker) SetNotifier(notifier notifications.Broker) {
	w.notifier = notifier
}

func clampString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}

func (w *PrescriptionOCRWorker) Work(ctx context.Context, job *river.Job[PrescriptionOCRArgs]) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	log.Printf("[River Worker] Processing OCR for prescription %s (key: %s)", job.Args.PrescriptionID, job.Args.StorageKey)

	attempt := 1
	if job != nil && job.JobRow != nil && job.Attempt > 0 {
		attempt = job.Attempt
	}

	// 1. If OCR is disabled, cleanly transition to needs_review for manual clinician entry
	if w.ocrManager == nil || !w.ocrManager.IsEnabled() {
		log.Printf("[River Worker] OCR disabled. Transitioning prescription %s to needs_review", job.Args.PrescriptionID)
		return w.repo.UpdatePrescriptionOCRResults(ctx, job.Args.PrescriptionID, "needs_review", "[OCR disabled: manual entry required]", "", nil)
	}

	// 2. Fetch file bytes from storage provider
	fileBytes, mimeType, err := w.storage.GetFileBytes(ctx, job.Args.StorageKey)
	if err != nil {
		isPermanent := errors.Is(err, storage.ErrFileTooLarge) ||
			errors.Is(err, os.ErrNotExist) ||
			strings.Contains(strings.ToLower(err.Error()), "not found") ||
			strings.Contains(strings.ToLower(err.Error()), "nosuchkey") ||
			strings.Contains(err.Error(), "404")

		if isPermanent {
			log.Printf("[River Worker] Permanent storage failure for prescription %s: %v. Marking needs_review", job.Args.PrescriptionID, err)
			if updateErr := w.repo.UpdatePrescriptionOCRResults(ctx, job.Args.PrescriptionID, "needs_review", fmt.Sprintf("[Storage error: %v]", err), "", nil); updateErr != nil {
				return fmt.Errorf("failed to update prescription status after permanent storage failure: %w", updateErr)
			}
			return nil
		}

		if attempt >= 3 {
			log.Printf("[River Worker] Max retry attempts reached for storage retrieval of prescription %s. Marking needs_review", job.Args.PrescriptionID)
			if updateErr := w.repo.UpdatePrescriptionOCRResults(ctx, job.Args.PrescriptionID, "needs_review", fmt.Sprintf("[Storage failed after %d attempts: %v]", attempt, err), "", nil); updateErr != nil {
				return fmt.Errorf("failed to update prescription status after max storage attempts: %w", updateErr)
			}
			return nil
		}

		log.Printf("[River Worker] Transient storage failure for prescription %s: %v. Retrying via River", job.Args.PrescriptionID, err)
		return err
	}

	// 3. Execute OCR extraction with fallback chain
	res, err := w.ocrManager.ExtractPrescription(ctx, fileBytes, mimeType)
	if err != nil {
		if errors.Is(err, ocr.ErrUnsupportedMIME) || errors.Is(err, ocr.ErrPermanent) {
			log.Printf("[River Worker] Permanent unrecoverable OCR failure for prescription %s: %v. Marking needs_review", job.Args.PrescriptionID, err)
			if updateErr := w.repo.UpdatePrescriptionOCRResults(ctx, job.Args.PrescriptionID, "needs_review", fmt.Sprintf("[OCR Extraction failed: %v]", err), "", nil); updateErr != nil {
				return fmt.Errorf("failed to update prescription status after permanent OCR failure: %w", updateErr)
			}
			w.notifyDoctor(ctx, job.Args.PrescriptionID, "Prescription OCR could not extract text; manual review required", 0, "")
			return nil // Fatal: return nil to avoid burning River retry attempts
		}
		if attempt >= 3 {
			log.Printf("[River Worker] Max retry attempts reached for prescription %s. Marking needs_review", job.Args.PrescriptionID)
			if updateErr := w.repo.UpdatePrescriptionOCRResults(ctx, job.Args.PrescriptionID, "needs_review", fmt.Sprintf("[OCR failed after %d attempts: %v]", attempt, err), "", nil); updateErr != nil {
				return fmt.Errorf("failed to update prescription status after max attempts: %w", updateErr)
			}
			w.notifyDoctor(ctx, job.Args.PrescriptionID, "Prescription OCR failed after max retries; manual review required", 0, "")
			return nil
		}
		// Transient failure: return error to River for exponential backoff retry
		log.Printf("[River Worker] Transient OCR error for prescription %s: %v. Returning to River for retry", job.Args.PrescriptionID, err)
		return err
	}

	// 4. Map extracted medications to models.PrescriptionItem (clamped to database column limits)
	items := make([]models.PrescriptionItem, 0, len(res.Medications))
	for _, med := range res.Medications {
		medName := clampString(med.MedicationName, 255)
		if medName == "" {
			continue
		}
		items = append(items, models.PrescriptionItem{
			MedicationName: medName,
			Dosage:         clampString(med.Dosage, 100),
			Frequency:      clampString(med.Frequency, 100),
			Duration:       clampString(med.Duration, 100),
			Timing:         clampString(med.Timing, 100),
			Instructions:   strings.TrimSpace(med.Instructions),
		})
	}

	// 5. Update database record with extracted items and transition to needs_review
	notes := ""
	if len(items) == 0 {
		notes = "[OCR Completed: No medications detected, manual review required]"
	}
	err = w.repo.UpdatePrescriptionOCRResults(ctx, job.Args.PrescriptionID, "needs_review", notes, clampString(res.Provider, 50), items)
	if err != nil {
		log.Printf("[River Worker] Failed to update prescription %s with OCR results: %v", job.Args.PrescriptionID, err)
		return err // Transient DB error, retry
	}

	log.Printf("[River Worker] Successfully extracted %d medications for prescription %s via %s. Status: needs_review", len(items), job.Args.PrescriptionID, res.Provider)

	w.notifyDoctor(ctx, job.Args.PrescriptionID, "Prescription OCR analysis ready for clinician review", len(items), res.Provider)
	return nil
}

func (w *PrescriptionOCRWorker) notifyDoctor(ctx context.Context, prescriptionID uuid.UUID, message string, medCount int, provider string) {
	if w.notifier == nil {
		return
	}
	rx, fetchErr := w.repo.GetPrescriptionByID(ctx, prescriptionID)
	if fetchErr != nil {
		return
	}
	data := map[string]interface{}{
		"medications_count": medCount,
	}
	if provider != "" {
		data["provider"] = provider
	}
	w.notifier.Publish(notifications.NotificationEvent{
		Type:           notifications.EventOCRCompleted,
		PrescriptionID: prescriptionID,
		DoctorID:       rx.DoctorID,
		Message:        message,
		Data:           data,
	})
}

// Client wraps river.Client for job queue management
type Client struct {
	RiverClient *river.Client[pgx.Tx]
}

// NewClient initializes a new River client with workers
func NewClient(pool *pgxpool.Pool, worker *PrescriptionOCRWorker) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create river client: %w", err)
	}

	return &Client{RiverClient: client}, nil
}

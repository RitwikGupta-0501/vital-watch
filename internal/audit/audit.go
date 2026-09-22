package audit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
)

// Standardized HIPAA Audit Action Types
const (
	ActionReadPrescriptions        = "READ_PRESCRIPTIONS"
	ActionDownloadPrescriptionFile = "DOWNLOAD_PRESCRIPTION_FILE"
	ActionExportPrescriptionPDF    = "EXPORT_PRESCRIPTION_PDF"
	ActionCreatePrescription       = "CREATE_PRESCRIPTION"
	ActionVerifyPrescription       = "VERIFY_PRESCRIPTION"
	ActionViewVitals               = "VIEW_VITALS"
	ActionRecordVitals             = "RECORD_VITALS"
	ActionViewMedicationSchedule   = "VIEW_MEDICATION_SCHEDULE"
	ActionLogMedicationAdherence   = "LOG_MEDICATION_ADHERENCE"
	ActionAccessMeetingRoom        = "ACCESS_MEETING_ROOM"
	ActionViewPatientProfile       = "VIEW_PATIENT_PROFILE"
)

type Auditor interface {
	Log(entry models.PhiAuditLog)
	Shutdown(ctx context.Context) error
}

// AsyncAuditor records audit logs in a non-blocking background queue
type AsyncAuditor struct {
	repo       repository.Repository
	logChan    chan models.PhiAuditLog
	wg         sync.WaitGroup
	bufferSize int
	closed     bool
	mu         sync.Mutex
}

func NewAsyncAuditor(repo repository.Repository, bufferSize int) *AsyncAuditor {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	a := &AsyncAuditor{
		repo:       repo,
		logChan:    make(chan models.PhiAuditLog, bufferSize),
		bufferSize: bufferSize,
	}

	a.wg.Add(1)
	go a.worker()
	return a
}

func (a *AsyncAuditor) worker() {
	defer a.wg.Done()
	for entry := range a.logChan {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if _, err := a.repo.CreateAuditLog(ctx, entry); err != nil {
			slog.Error("Failed to persist HIPAA ePHI audit log", "error", err, "action", entry.Action)
		}
		cancel()
	}
}

func (a *AsyncAuditor) Log(entry models.PhiAuditLog) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	select {
	case a.logChan <- entry:
	default:
		slog.Warn("HIPAA Audit queue full; logging synchronously to prevent audit loss", "action", entry.Action)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = a.repo.CreateAuditLog(ctx, entry)
	}
}

func (a *AsyncAuditor) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	close(a.logChan)
	a.mu.Unlock()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// MockAuditor for testing
type MockAuditor struct {
	mu      sync.Mutex
	Entries []models.PhiAuditLog
}

func NewMockAuditor() *MockAuditor {
	return &MockAuditor{
		Entries: make([]models.PhiAuditLog, 0),
	}
}

func (m *MockAuditor) Log(entry models.PhiAuditLog) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Entries = append(m.Entries, entry)
}

func (m *MockAuditor) Shutdown(ctx context.Context) error {
	return nil
}

func (m *MockAuditor) GetEntries() []models.PhiAuditLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]models.PhiAuditLog, len(m.Entries))
	copy(copied, m.Entries)
	return copied
}

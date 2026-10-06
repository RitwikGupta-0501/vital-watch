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
	// Clinical Appointments & Scheduling Events (HIPAA § 164.312(b))
	ActionCreateAppointment   = "CREATE_APPOINTMENT"
	ActionCompleteAppointment = "COMPLETE_APPOINTMENT"
	ActionCancelAppointment   = "CANCEL_APPOINTMENT"
	ActionViewAppointments    = "VIEW_APPOINTMENTS"
	ActionUpdateSchedule      = "UPDATE_DOCTOR_SCHEDULE"
	ActionViewPatientProfile       = "VIEW_PATIENT_PROFILE"
	ActionViewDoctorProfile        = "VIEW_DOCTOR_PROFILE"
	ActionViewAdminProfile         = "VIEW_ADMIN_PROFILE"
	// Authentication & Security Audit Events (HIPAA § 164.312(b))
	ActionUserRegister        = "USER_REGISTER"
	ActionUserLogin           = "USER_LOGIN"
	ActionUserLogout          = "USER_LOGOUT"
	ActionTokenReuseAlert     = "TOKEN_REUSE_SECURITY_ALERT"
	ActionRateLimitExceeded   = "RATE_LIMIT_EXCEEDED"
	// Administration & Governance Audit Events (HIPAA § 164.312(a)(2)(i))
	ActionViewUserDirectory = "VIEW_USER_DIRECTORY"
	ActionToggleUserStatus  = "TOGGLE_USER_STATUS"
	// Compliance Audit Trail Access
	ActionViewAuditLogs = "VIEW_COMPLIANCE_AUDIT_LOGS"
)

// DefaultWorkerCount is the number of concurrent goroutines draining the audit queue.
const DefaultWorkerCount = 5

type Auditor interface {
	Log(entry models.PhiAuditLog)
	Shutdown(ctx context.Context) error
}

type DLQ interface {
	Enqueue(ctx context.Context, entry models.PhiAuditLog) error
}

// AsyncAuditor records audit logs in a non-blocking background queue using a
// worker pool to prevent single-goroutine throughput bottlenecks.
type AsyncAuditor struct {
	repo        repository.Repository
	dlq         DLQ
	logChan     chan models.PhiAuditLog
	wg          sync.WaitGroup
	workerCount int
	mu          sync.RWMutex
	closed      bool
}

func NewAsyncAuditor(repo repository.Repository, dlq DLQ, bufferSize int) *AsyncAuditor {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	a := &AsyncAuditor{
		repo:        repo,
		dlq:         dlq,
		logChan:     make(chan models.PhiAuditLog, bufferSize),
		workerCount: DefaultWorkerCount,
	}

	for i := 0; i < a.workerCount; i++ {
		a.wg.Add(1)
		go a.worker()
	}
	return a
}

func (a *AsyncAuditor) worker() {
	defer a.wg.Done()
	for entry := range a.logChan {
		var err error
		backoff := 100 * time.Millisecond
		maxRetries := 3

		for i := 0; i <= maxRetries; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err = a.repo.CreateAuditLog(ctx, entry)
			cancel()

			if err == nil {
				break
			}
			if i < maxRetries {
				time.Sleep(backoff)
				backoff *= 2
			}
		}

		if err != nil {
			slog.Error("Failed to persist HIPAA ePHI audit log after retries. Routing to DLQ.", "error", err, "action", entry.Action)
			if a.dlq != nil {
				dlqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if dlqErr := a.dlq.Enqueue(dlqCtx, entry); dlqErr != nil {
					slog.Error("CRITICAL: Failed to enqueue audit log to DLQ. Data lost.", "error", dlqErr, "action", entry.Action)
				}
				cancel()
			}
		}
	}
}

func (a *AsyncAuditor) Log(entry models.PhiAuditLog) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return
	}

	select {
	case a.logChan <- entry:
		a.mu.RUnlock()
		return
	default:
		a.mu.RUnlock()
		slog.Warn("HIPAA Audit queue full; routing directly to DLQ to prevent blocking", "action", entry.Action)
		
		if a.dlq != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := a.dlq.Enqueue(ctx, entry); err != nil {
				slog.Error("Critical: Failed to persist audit log to DLQ on queue full fallback", "error", err, "action", entry.Action)
			}
		}
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


// MockAuditor provides an in-memory thread-safe implementation of Auditor for testing.
type MockAuditor struct {
	mu      sync.Mutex
	entries []models.PhiAuditLog
}

func NewMockAuditor() *MockAuditor {
	return &MockAuditor{
		entries: make([]models.PhiAuditLog, 0),
	}
}

func (m *MockAuditor) Log(entry models.PhiAuditLog) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
}

func (m *MockAuditor) Shutdown(ctx context.Context) error {
	return nil
}

func (m *MockAuditor) GetEntries() []models.PhiAuditLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.PhiAuditLog, len(m.entries))
	copy(out, m.entries)
	return out
}

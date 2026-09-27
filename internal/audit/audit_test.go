package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
)

type MockDLQ struct {
	mu      sync.Mutex
	entries []models.PhiAuditLog
}

func (m *MockDLQ) Enqueue(ctx context.Context, entry models.PhiAuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func TestAsyncAuditor_WorkerAndDrain(t *testing.T) {
	var loggedEntries []models.PhiAuditLog
	mockRepo := &repository.MockRepository{
		CreateAuditLogFunc: func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
			loggedEntries = append(loggedEntries, log)
			return uuid.New(), nil
		},
	}

	dlq := &MockDLQ{}
	auditor := NewAsyncAuditor(mockRepo, dlq, 10)

	patientID := uuid.New()
	userID := uuid.New()
	reqID := uuid.New()

	entry := models.PhiAuditLog{
		UserID:       &userID,
		UserRole:     "doctor",
		Action:       ActionReadPrescriptions,
		ResourceType: "prescription",
		PatientID:    &patientID,
		RequestID:    &reqID,
		StatusCode:   200,
	}

	auditor.Log(entry)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := auditor.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("unexpected error during shutdown: %v", err)
	}

	if len(loggedEntries) != 1 {
		t.Fatalf("expected 1 logged audit entry, got %d", len(loggedEntries))
	}
	if loggedEntries[0].Action != ActionReadPrescriptions {
		t.Errorf("expected action %s, got %s", ActionReadPrescriptions, loggedEntries[0].Action)
	}
}

func TestMockAuditor(t *testing.T) {
	mock := NewMockAuditor()
	mock.Log(models.PhiAuditLog{Action: ActionViewVitals})

	entries := mock.GetEntries()
	if len(entries) != 1 || entries[0].Action != ActionViewVitals {
		t.Errorf("mock auditor failed to record entry")
	}
}

func TestAsyncAuditor_ConcurrentLogAndShutdown(t *testing.T) {
	mockRepo := &repository.MockRepository{
		CreateAuditLogFunc: func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
			return uuid.New(), nil
		},
	}

	dlq := &MockDLQ{}
	auditor := NewAsyncAuditor(mockRepo, dlq, 100)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			auditor.Log(models.PhiAuditLog{Action: ActionReadPrescriptions})
			time.Sleep(100 * time.Microsecond)
		}
		close(done)
	}()

	time.Sleep(5 * time.Millisecond)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = auditor.Shutdown(shutdownCtx)

	for i := 0; i < 50; i++ {
		auditor.Log(models.PhiAuditLog{Action: ActionReadPrescriptions})
	}
	<-done
}

func TestAsyncAuditor_HighConcurrencyStress(t *testing.T) {
	var count int64
	var mu sync.Mutex
	mockRepo := &repository.MockRepository{
		CreateAuditLogFunc: func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
			mu.Lock()
			count++
			mu.Unlock()
			return uuid.New(), nil
		},
	}

	dlq := &MockDLQ{}
	auditor := NewAsyncAuditor(mockRepo, dlq, 50)
	concurrency := 20
	iterations := 25
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				auditor.Log(models.PhiAuditLog{Action: ActionRecordVitals})
			}
		}()
	}

	wg.Wait()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := auditor.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	mu.Lock()
	finalCount := count
	mu.Unlock()

	dlq.mu.Lock()
	dlqCount := len(dlq.entries)
	dlq.mu.Unlock()

	expectedTotal := concurrency * iterations
	if int(finalCount)+dlqCount != expectedTotal {
		t.Errorf("expected %d entries, got %d (DB) + %d (DLQ)", expectedTotal, finalCount, dlqCount)
	}
}

func TestAsyncAuditor_WorkerRetriesAndDLQ(t *testing.T) {
	var attempts int
	var mu sync.Mutex
	mockRepo := &repository.MockRepository{
		CreateAuditLogFunc: func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
			mu.Lock()
			attempts++
			mu.Unlock()
			return uuid.Nil, errors.New("simulated persistent database failure")
		},
	}

	dlq := &MockDLQ{}
	auditor := NewAsyncAuditor(mockRepo, dlq, 10)

	auditor.Log(models.PhiAuditLog{Action: ActionLogMedicationAdherence})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := auditor.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("auditor failed to shutdown: %v", err)
	}

	mu.Lock()
	finalAttempts := attempts
	mu.Unlock()

	// Initial attempt + 3 retries = 4 attempts total
	if finalAttempts != 4 {
		t.Errorf("expected 4 repo creation attempts, got %d", finalAttempts)
	}

	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	if len(dlq.entries) != 1 {
		t.Fatalf("expected 1 entry in DLQ, got %d", len(dlq.entries))
	}
	if dlq.entries[0].Action != ActionLogMedicationAdherence {
		t.Errorf("expected DLQ action %s, got %s", ActionLogMedicationAdherence, dlq.entries[0].Action)
	}
}

func TestAsyncAuditor_QueueFullRoutesToDLQ(t *testing.T) {
	var count int64
	var mu sync.Mutex
	mockRepo := &repository.MockRepository{
		CreateAuditLogFunc: func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
			// Simulate a slow database to force queue to fill
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			count++
			mu.Unlock()
			return uuid.New(), nil
		},
	}

	dlq := &MockDLQ{}
	// Tiny buffer of 1 to quickly force DLQ fallback
	auditor := NewAsyncAuditor(mockRepo, dlq, 1)

	// Send 10 entries rapidly. The queue (size 1) and workers (5) will consume up to 6 immediately.
	// The rest should instantly fall back to the DLQ.
	totalEntries := 10
	for i := 0; i < totalEntries; i++ {
		auditor.Log(models.PhiAuditLog{Action: ActionViewVitals})
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = auditor.Shutdown(shutdownCtx)

	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	// We expect some entries to have been routed to the DLQ since the DB was slow.
	if len(dlq.entries) == 0 {
		t.Errorf("expected some entries to route to DLQ due to full queue, got 0")
	}

	// The total processed by DB + DLQ should equal the total entries sent
	mu.Lock()
	finalCount := count
	mu.Unlock()
	
	if int(finalCount)+len(dlq.entries) != totalEntries {
		t.Errorf("expected total entries %d, got %d (DB) + %d (DLQ)", totalEntries, finalCount, len(dlq.entries))
	}
}

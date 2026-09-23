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

func TestAsyncAuditor_WorkerAndDrain(t *testing.T) {
	var loggedEntries []models.PhiAuditLog
	mockRepo := &repository.MockRepository{
		CreateAuditLogFunc: func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
			loggedEntries = append(loggedEntries, log)
			return uuid.New(), nil
		},
	}

	auditor := NewAsyncAuditor(mockRepo, 10)

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

	// Shutdown should drain pending logs
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

	auditor := NewAsyncAuditor(mockRepo, 100)

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

	// Additional Log calls after shutdown should be safely ignored and not panic
	for i := 0; i < 50; i++ {
		auditor.Log(models.PhiAuditLog{Action: ActionReadPrescriptions})
	}
	<-done
}

func TestAsyncAuditor_QueueFullFallback(t *testing.T) {
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

	// Tiny buffer of 2
	auditor := NewAsyncAuditor(mockRepo, 2)

	// Send 15 items rapidly, which will fill buffer of 2 and trigger synchronous fallback
	totalEntries := 15
	for i := 0; i < totalEntries; i++ {
		auditor.Log(models.PhiAuditLog{Action: ActionViewVitals})
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := auditor.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	mu.Lock()
	finalCount := count
	mu.Unlock()

	if int(finalCount) != totalEntries {
		t.Errorf("expected all %d entries to be logged via async queue or fallback, got %d", totalEntries, finalCount)
	}
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

	auditor := NewAsyncAuditor(mockRepo, 50)
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

	expectedTotal := concurrency * iterations
	if int(finalCount) != expectedTotal {
		t.Errorf("expected %d entries, got %d", expectedTotal, finalCount)
	}
}

func TestAsyncAuditor_RepoErrorResilience(t *testing.T) {
	mockRepo := &repository.MockRepository{
		CreateAuditLogFunc: func(ctx context.Context, log models.PhiAuditLog) (uuid.UUID, error) {
			return uuid.Nil, errors.New("simulated database failure")
		},
	}

	auditor := NewAsyncAuditor(mockRepo, 10)

	for i := 0; i < 5; i++ {
		auditor.Log(models.PhiAuditLog{Action: ActionLogMedicationAdherence})
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := auditor.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("auditor failed to shutdown cleanly after repo error: %v", err)
	}
}

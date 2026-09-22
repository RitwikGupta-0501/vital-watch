package audit

import (
	"context"
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

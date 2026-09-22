package notifications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestSSEBroker_SubscribeAndPublish(t *testing.T) {
	broker := NewSSEBroker()
	defer broker.Shutdown()

	doctorID := uuid.New()
	patientID := uuid.New()

	chDoc, unsubDoc := broker.Subscribe(doctorID)
	defer unsubDoc()

	chPat, unsubPat := broker.Subscribe(patientID)
	defer unsubPat()

	// 1. Publish event targeting doctor only
	rxID := uuid.New()
	broker.Publish(NotificationEvent{
		Type:           EventOCRCompleted,
		PrescriptionID: rxID,
		DoctorID:       doctorID,
		Message:        "Prescription OCR analysis ready for review",
	})

	select {
	case event := <-chDoc:
		if event.Type != EventOCRCompleted || event.PrescriptionID != rxID {
			t.Errorf("unexpected event received by doctor: %+v", event)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for doctor event")
	}

	// Verify patient did not receive the doctor-targeted event
	select {
	case event := <-chPat:
		t.Fatalf("patient received event intended only for doctor: %+v", event)
	default:
		// OK
	}

	// 2. Publish event targeting patient
	broker.Publish(NotificationEvent{
		Type:           EventPrescriptionApproved,
		PrescriptionID: rxID,
		PatientID:      patientID,
		Message:        "Your prescription has been approved by your doctor",
	})

	select {
	case event := <-chPat:
		if event.Type != EventPrescriptionApproved {
			t.Errorf("unexpected event received by patient: %+v", event)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for patient event")
	}
}

func TestSSEBroker_Shutdown(t *testing.T) {
	broker := NewSSEBroker()
	userID := uuid.New()

	ch, _ := broker.Subscribe(userID)
	broker.Shutdown()

	// After shutdown, channels should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("expected channel to be closed after shutdown")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("channel was not closed on shutdown")
	}
}

func TestServeSSE_InitialConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	broker := NewSSEBroker()
	defer broker.Shutdown()

	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/notifications/stream", nil)
	c.Request = req

	// Cancel context after 50ms so ServeSSE returns cleanly
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	ServeSSE(broker, c, userID)

	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", w.Header().Get("Content-Type"))
	}
}

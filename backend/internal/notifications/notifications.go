package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventType string

const (
	EventOCRCompleted         EventType = "prescription.ocr_completed"
	EventPrescriptionApproved EventType = "prescription.approved"
	EventPrescriptionRejected EventType = "prescription.rejected"
	EventAppointmentBooked    EventType = "appointment.booked"
	EventAppointmentCompleted EventType = "appointment.completed"
	EventAppointmentCancelled EventType = "appointment.cancelled"
	EventVitalAlert           EventType = "vital.alert"
	EventMedicationLogged     EventType = "medication.logged"
	EventPing                 EventType = "ping"
)

type NotificationEvent struct {
	Type           EventType   `json:"type"`
	PrescriptionID uuid.UUID   `json:"prescription_id,omitempty"`
	AppointmentID  uuid.UUID   `json:"appointment_id,omitempty"`
	PatientID      uuid.UUID   `json:"patient_id,omitempty"`
	DoctorID       uuid.UUID   `json:"doctor_id,omitempty"`
	Message        string      `json:"message"`
	Data           interface{} `json:"data,omitempty"`
	Timestamp      time.Time   `json:"timestamp"`
}

type Broker interface {
	Subscribe(userID uuid.UUID) (chan NotificationEvent, func())
	Publish(event NotificationEvent)
	Shutdown()
}

type SSEBroker struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[chan NotificationEvent]struct{}
	closed  bool
	pool    *pgxpool.Pool
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewSSEBroker(pool *pgxpool.Pool) *SSEBroker {
	ctx, cancel := context.WithCancel(context.Background())
	b := &SSEBroker{
		clients: make(map[uuid.UUID]map[chan NotificationEvent]struct{}),
		pool:    pool,
		ctx:     ctx,
		cancel:  cancel,
	}

	if pool != nil {
		go b.listenForNotifications()
	}

	return b
}

func (b *SSEBroker) listenForNotifications() {
	for {
		if b.ctx.Err() != nil {
			return
		}

		conn, err := b.pool.Acquire(b.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Error("Failed to acquire pgx connection for LISTEN", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		_, err = conn.Exec(b.ctx, "LISTEN sse_notifications")
		if err != nil {
			conn.Release()
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Error("Failed to execute LISTEN", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		slog.Info("SSE Broker connected to Postgres LISTEN/NOTIFY channel")

		for {
			// Use a timeout to prevent silent TCP half-open connection drops
			waitCtx, cancelWait := context.WithTimeout(b.ctx, 30*time.Second)
			notification, err := conn.Conn().WaitForNotification(waitCtx)
			cancelWait()

			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					// Ping to ensure connection is still alive
					if pingErr := conn.Ping(b.ctx); pingErr != nil {
						slog.Error("Postgres LISTEN connection dead, reconnecting...", "error", pingErr)
						conn.Release()
						break // re-acquire connection
					}
					continue
				}
				conn.Release()
				if errors.Is(err, context.Canceled) {
					return
				}
				slog.Error("Error waiting for pg_notify", "error", err)
				time.Sleep(2 * time.Second)
				break // break inner loop to re-acquire connection
			}

			var event NotificationEvent
			if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil {
				slog.Error("Failed to unmarshal notification payload", "error", err)
				continue
			}
			b.deliverLocal(event)
		}
	}
}

func (b *SSEBroker) Subscribe(userID uuid.UUID) (chan NotificationEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan NotificationEvent, 64)
	if b.closed {
		close(ch)
		return ch, func() {}
	}

	if _, exists := b.clients[userID]; !exists {
		b.clients[userID] = make(map[chan NotificationEvent]struct{})
	}
	b.clients[userID][ch] = struct{}{}

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subs, exists := b.clients[userID]; exists {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.clients, userID)
			}
		}
	}

	return ch, unsubscribe
}

func (b *SSEBroker) Publish(event NotificationEvent) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	b.mu.RUnlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	if b.pool != nil {
		dataBytes, _ := json.Marshal(event)
		if len(dataBytes) > 7900 {
			slog.Warn("SSE notification payload too large for Postgres NOTIFY, truncating Data field", "event_type", event.Type)
			event.Data = nil
			dataBytes, _ = json.Marshal(event)
		}

		ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
		defer cancel()
		_, err := b.pool.Exec(ctx, "SELECT pg_notify('sse_notifications', $1)", string(dataBytes))
		if err != nil {
			slog.Error("Failed to publish SSE notification to postgres", "error", err)
		}
		return
	}

	b.deliverLocal(event)
}

func (b *SSEBroker) deliverLocal(event NotificationEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	deliver := func(targetID uuid.UUID) {
		if targetID == uuid.Nil {
			return
		}
		if subs, ok := b.clients[targetID]; ok {
			for ch := range subs {
				select {
				case ch <- event:
				default:
					slog.Warn("Notification buffer full; event dropped", "user_id", targetID, "event_type", event.Type)
				}
			}
		}
	}

	// Deliver to both doctor and/or patient if targeted
	if event.DoctorID != uuid.Nil {
		deliver(event.DoctorID)
	}
	if event.PatientID != uuid.Nil && event.PatientID != event.DoctorID {
		deliver(event.PatientID)
	}
}

func (b *SSEBroker) Shutdown() {
	b.cancel() // Stop the listener goroutine

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for _, subs := range b.clients {
		for ch := range subs {
			close(ch)
		}
	}
	b.clients = make(map[uuid.UUID]map[chan NotificationEvent]struct{})
}

// ServeSSE handles the HTTP connection for Server-Sent Events
func ServeSSE(b Broker, c *gin.Context, userID uuid.UUID) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Disable HTTP server write deadline for persistent SSE stream
	rc := http.NewResponseController(c.Writer)
	_ = rc.SetWriteDeadline(time.Time{})

	eventCh, unsubscribe := b.Subscribe(userID)
	defer unsubscribe()

	// 15-second heartbeat ticker to keep reverse proxies alive
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Initial connected message
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: {\"connected\": true}\n\n", EventPing); err != nil {
		return
	}
	c.Writer.Flush()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			dataBytes, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, string(dataBytes)); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

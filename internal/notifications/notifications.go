package notifications

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type EventType string

const (
	EventOCRCompleted         EventType = "prescription.ocr_completed"
	EventPrescriptionApproved EventType = "prescription.approved"
	EventPrescriptionRejected EventType = "prescription.rejected"
	EventPing                 EventType = "ping"
)

type NotificationEvent struct {
	Type           EventType   `json:"type"`
	PrescriptionID uuid.UUID   `json:"prescription_id,omitempty"`
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
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[uuid.UUID]map[chan NotificationEvent]struct{}),
	}
}

func (b *SSEBroker) Subscribe(userID uuid.UUID) (chan NotificationEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan NotificationEvent, 16)
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
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
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
					log.Printf("[SSE Warning] Notification buffer full for user %s; event %s dropped", targetID, event.Type)
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

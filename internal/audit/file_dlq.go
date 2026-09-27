package audit

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

// FileDLQ is a simple thread-safe Dead Letter Queue that appends failed audit logs
// to a local JSON Lines (.jsonl) file.
type FileDLQ struct {
	mu       sync.Mutex
	filePath string
}

func NewFileDLQ(filePath string) *FileDLQ {
	return &FileDLQ{
		filePath: filePath,
	}
}

// Enqueue serializes the audit log to JSON and appends it to the file.
func (f *FileDLQ) Enqueue(ctx context.Context, entry models.PhiAuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Ensure we don't hold the lock forever if ctx cancels early, though file I/O is usually fast.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Make sure the entry has a timestamp if it somehow got lost
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n') // JSON Lines format

	// Open file in append mode, create if it doesn't exist
	file, err := os.OpenFile(f.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

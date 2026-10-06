package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
)

func main() {
	fmt.Println("[*] Connecting to PostgreSQL...")
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://appuser:apppassword@localhost:5432/appdb?sslmode=disable")
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Printf("Failed to ping: %v\n", err)
		return
	}

	// Create a large payload > 8000 bytes
	largeData := strings.Repeat("A", 8001)
	
	event := notifications.NotificationEvent{
		Type:    "prescription_ready",
		Message: "Your prescription is ready for review.",
		Data:    largeData,
	}

	fmt.Println("[*] Attempting to publish NotificationEvent with >8000 byte payload...")
	
	dataBytes, _ := json.Marshal(event)
	
	query := "SELECT pg_notify('sse_notifications', $1)"
	
	fmt.Printf("[*] Executing: SELECT pg_notify(..., <payload of length %d>)\n", len(dataBytes))
	_, err = pool.Exec(ctx, query, string(dataBytes))
	
	if err != nil {
		fmt.Printf("❌ BUG PROVED: pg_notify failed as expected.\n")
		fmt.Printf("   PostgreSQL Error: %v\n", err)
		fmt.Printf("   Explanation: PostgreSQL imposes a strict 8000-byte limit on NOTIFY payloads.\n")
	} else {
		fmt.Printf("✅ BUG NOT REPRODUCED: pg_notify succeeded.\n")
	}
}

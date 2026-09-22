package telehealth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJitsiProvider_CreateRoom(t *testing.T) {
	provider := NewJitsiProvider("meet.jit.si", "test-secret-salt")

	apptID := uuid.New()
	startTime := time.Now().Add(1 * time.Hour)
	duration := 30 * time.Minute

	room, err := provider.CreateRoom(context.Background(), apptID, startTime, duration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if room == nil {
		t.Fatal("expected room details, got nil")
	}

	if room.Provider != "jitsi" {
		t.Errorf("expected provider jitsi, got %s", room.Provider)
	}

	if !strings.HasPrefix(room.MeetingLink, "https://meet.jit.si/vitalwatch-") {
		t.Errorf("expected link starting with https://meet.jit.si/vitalwatch-, got %s", room.MeetingLink)
	}

	if !strings.HasPrefix(room.MeetingID, "vitalwatch-") {
		t.Errorf("expected id starting with vitalwatch-, got %s", room.MeetingID)
	}

	// Nil appointment ID should error
	_, err = provider.CreateRoom(context.Background(), uuid.Nil, startTime, duration)
	if err == nil {
		t.Error("expected error for nil appointment ID, got nil")
	}
}

func TestMockProvider(t *testing.T) {
	mock := &MockProvider{
		CustomID:   "custom-room-123",
		CustomLink: "https://test.local/room-123",
	}

	room, err := mock.CreateRoom(context.Background(), uuid.New(), time.Now(), 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if room.MeetingID != "custom-room-123" || room.MeetingLink != "https://test.local/room-123" {
		t.Errorf("mock provider did not return custom room details: %+v", room)
	}
}

func TestNewTelehealthManager_DefaultJitsi(t *testing.T) {
	mgr := NewTelehealthManager()
	if mgr == nil {
		t.Fatal("expected non-nil telehealth manager")
	}
	if mgr.Name() != "jitsi" {
		t.Errorf("expected default provider to be jitsi, got %s", mgr.Name())
	}
}

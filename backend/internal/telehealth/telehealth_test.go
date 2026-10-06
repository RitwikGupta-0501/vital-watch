package telehealth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestDailyProvider_CreateRoomAndToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		if r.URL.Path == "/rooms" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "daily-room-test-123",
				"url":  "https://daily.co/daily-room-test-123",
			})
			return
		}

		if r.URL.Path == "/meeting-tokens" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"token": "daily-meeting-jwt-token-xyz",
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Run("CreateRoom Success", func(t *testing.T) {
		provider := NewDailyProvider("valid-test-key", server.URL)
		room, err := provider.CreateRoom(context.Background(), uuid.New(), time.Now(), 30*time.Minute)
		if err != nil {
			t.Fatalf("unexpected error creating room: %v", err)
		}
		if room.MeetingID != "daily-room-test-123" {
			t.Errorf("expected meeting id daily-room-test-123, got %s", room.MeetingID)
		}
		if room.MeetingLink != "https://daily.co/daily-room-test-123" {
			t.Errorf("expected link https://daily.co/daily-room-test-123, got %s", room.MeetingLink)
		}
		if room.Provider != "daily" {
			t.Errorf("expected provider daily, got %s", room.Provider)
		}
	})

	t.Run("CreateMeetingToken Success", func(t *testing.T) {
		provider := NewDailyProvider("valid-test-key", server.URL)
		token, err := provider.CreateMeetingToken(context.Background(), "daily-room-test-123", true, time.Now().Add(1*time.Hour))
		if err != nil {
			t.Fatalf("unexpected error creating token: %v", err)
		}
		if token != "daily-meeting-jwt-token-xyz" {
			t.Errorf("expected token daily-meeting-jwt-token-xyz, got %s", token)
		}
	})

	t.Run("Unauthorized 401 Error Handling", func(t *testing.T) {
		provider := NewDailyProvider("invalid-key", server.URL)
		_, err := provider.CreateRoom(context.Background(), uuid.New(), time.Now(), 30*time.Minute)
		if err == nil {
			t.Fatalf("expected error on 401 unauthorized, got nil")
		}
		if !strings.Contains(err.Error(), "401") {
			t.Errorf("expected error to mention 401, got: %v", err)
		}

		_, tokErr := provider.CreateMeetingToken(context.Background(), "room", false, time.Now())
		if tokErr == nil || !strings.Contains(tokErr.Error(), "401") {
			t.Errorf("expected meeting token 401 error, got: %v", tokErr)
		}
	})

	t.Run("Unconfigured API Key", func(t *testing.T) {
		provider := NewDailyProvider("")
		_, err := provider.CreateRoom(context.Background(), uuid.New(), time.Now(), 30*time.Minute)
		if err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Errorf("expected not configured error, got: %v", err)
		}

		_, tokErr := provider.CreateMeetingToken(context.Background(), "room", false, time.Now())
		if tokErr == nil || !strings.Contains(tokErr.Error(), "not configured") {
			t.Errorf("expected not configured error, got: %v", tokErr)
		}
	})
}

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

func TestJitsiProvider_CreateMeetingToken(t *testing.T) {
	secret := "super-secure-jitsi-secret-key"
	domain := "telehealth.vitalwatch.org"
	provider := NewJitsiProvider(domain, secret)

	roomName := "vitalwatch-test-room-101"
	exp := time.Now().Add(1 * time.Hour)

	t.Run("CreateMeetingToken Doctor Owner", func(t *testing.T) {
		tokenStr, err := provider.CreateMeetingToken(context.Background(), roomName, true, exp)
		if err != nil {
			t.Fatalf("unexpected error creating doctor meeting token: %v", err)
		}
		if tokenStr == "" {
			t.Fatal("expected non-empty token string")
		}

		// Parse and validate JWT
		parsedToken, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})
		if err != nil || !parsedToken.Valid {
			t.Fatalf("failed to validate generated jitsi token: %v", err)
		}

		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		if !ok {
			t.Fatal("expected MapClaims")
		}

		if claims["room"] != roomName {
			t.Errorf("expected room %s, got %v", roomName, claims["room"])
		}
		if claims["sub"] != domain {
			t.Errorf("expected sub %s, got %v", domain, claims["sub"])
		}
		if claims["iss"] != "vitalwatch" {
			t.Errorf("expected iss vitalwatch, got %v", claims["iss"])
		}
		if claims["aud"] != "jitsi" {
			t.Errorf("expected aud jitsi, got %v", claims["aud"])
		}

		ctxMap, ok := claims["context"].(map[string]interface{})
		if !ok {
			t.Fatal("expected context object in claims")
		}
		userMap, ok := ctxMap["user"].(map[string]interface{})
		if !ok {
			t.Fatal("expected user object in context claims")
		}
		if userMap["affiliation"] != "owner" || userMap["moderator"] != true {
			t.Errorf("expected owner/moderator privileges for doctor, got: %+v", userMap)
		}
	})

	t.Run("CreateMeetingToken Patient Member", func(t *testing.T) {
		tokenStr, err := provider.CreateMeetingToken(context.Background(), roomName, false, exp)
		if err != nil {
			t.Fatalf("unexpected error creating patient meeting token: %v", err)
		}

		parsedToken, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !parsedToken.Valid {
			t.Fatalf("failed to validate generated jitsi token: %v", err)
		}

		claims := parsedToken.Claims.(jwt.MapClaims)
		ctxMap := claims["context"].(map[string]interface{})
		userMap := ctxMap["user"].(map[string]interface{})
		if userMap["affiliation"] != "member" || userMap["moderator"] != false {
			t.Errorf("expected member/non-moderator privileges for patient, got: %+v", userMap)
		}
	})
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

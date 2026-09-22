package telehealth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type RoomDetails struct {
	MeetingID   string `json:"meeting_id"`
	MeetingLink string `json:"meeting_link"`
	Provider    string `json:"provider"`
}

type Provider interface {
	Name() string
	CreateRoom(ctx context.Context, appointmentID uuid.UUID, startTime time.Time, duration time.Duration) (*RoomDetails, error)
}

type TokenProvider interface {
	CreateMeetingToken(ctx context.Context, roomName string, isOwner bool, exp time.Time) (string, error)
}

// JitsiProvider provides zero-dependency video conferencing via Jitsi Meet
type JitsiProvider struct {
	domain string
	secret string
}

func NewJitsiProvider(domain, secret string) *JitsiProvider {
	d := strings.TrimSpace(domain)
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimRight(d, "/")
	if d == "" {
		d = "meet.jit.si"
	}
	s := strings.TrimSpace(secret)
	if s == "" {
		randBytes := make([]byte, 16)
		_, _ = rand.Read(randBytes)
		s = hex.EncodeToString(randBytes)
	}
	return &JitsiProvider{domain: d, secret: s}
}

func (j *JitsiProvider) Name() string {
	return "jitsi"
}

func (j *JitsiProvider) CreateRoom(ctx context.Context, appointmentID uuid.UUID, startTime time.Time, duration time.Duration) (*RoomDetails, error) {
	if appointmentID == uuid.Nil {
		return nil, fmt.Errorf("appointment ID is required")
	}

	// Generate deterministic, cryptographically salted room name
	mac := hmac.New(sha256.New, []byte(j.secret))
	mac.Write([]byte(fmt.Sprintf("%s-%d", appointmentID.String(), startTime.Unix())))
	hashStr := hex.EncodeToString(mac.Sum(nil))[:16]

	roomName := fmt.Sprintf("vitalwatch-%s-%s", appointmentID.String()[:8], hashStr)
	roomLink := fmt.Sprintf("https://%s/%s", j.domain, roomName)

	return &RoomDetails{
		MeetingID:   roomName,
		MeetingLink: roomLink,
		Provider:    j.Name(),
	}, nil
}

// DailyProvider creates ephemeral video rooms using the Daily.co REST API
type DailyProvider struct {
	apiKey     string
	httpClient *http.Client
}

func NewDailyProvider(apiKey string) *DailyProvider {
	return &DailyProvider{
		apiKey: strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (d *DailyProvider) Name() string {
	return "daily"
}

func (d *DailyProvider) CreateRoom(ctx context.Context, appointmentID uuid.UUID, startTime time.Time, duration time.Duration) (*RoomDetails, error) {
	if d.apiKey == "" {
		return nil, fmt.Errorf("daily API key is not configured")
	}

	exp := startTime.Add(duration + 2*time.Hour).Unix()
	if minExp := time.Now().Add(1 * time.Hour).Unix(); exp < minExp {
		exp = minExp
	}
	reqBody := map[string]interface{}{
		"privacy": "private",
		"properties": map[string]interface{}{
			"exp":                exp,
			"enable_chat":        true,
			"enable_screenshare": true,
			"start_video_off":    false,
			"start_audio_off":    false,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.daily.co/v1/rooms", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call daily API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("daily API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBytes)))
	}

	var res struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode daily API response: %w", err)
	}

	return &RoomDetails{
		MeetingID:   res.Name,
		MeetingLink: res.URL,
		Provider:    d.Name(),
	}, nil
}

func (d *DailyProvider) CreateMeetingToken(ctx context.Context, roomName string, isOwner bool, exp time.Time) (string, error) {
	if d.apiKey == "" {
		return "", fmt.Errorf("daily API key is not configured")
	}
	expUnix := exp.Unix()
	if minExp := time.Now().Add(30 * time.Minute).Unix(); expUnix < minExp {
		expUnix = minExp
	}
	reqBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"room_name": roomName,
			"is_owner":  isOwner,
			"exp":       expUnix,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.daily.co/v1/meeting-tokens", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call daily meeting-tokens API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("daily meeting-tokens API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBytes)))
	}

	var res struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("failed to decode daily token response: %w", err)
	}
	return res.Token, nil
}

// MockProvider is used for unit testing
type MockProvider struct {
	CustomLink string
	CustomID   string
	Err        error
}

func (m *MockProvider) CreateMeetingToken(ctx context.Context, roomName string, isOwner bool, exp time.Time) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if isOwner {
		return "mock-owner-token", nil
	}
	return "mock-patient-token", nil
}

func (m *MockProvider) Name() string {
	return "mock"
}

func (m *MockProvider) CreateRoom(ctx context.Context, appointmentID uuid.UUID, startTime time.Time, duration time.Duration) (*RoomDetails, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	id := m.CustomID
	if id == "" {
		id = "mock-room-" + appointmentID.String()
	}
	link := m.CustomLink
	if link == "" {
		link = "https://mock.telehealth.local/" + id
	}
	return &RoomDetails{
		MeetingID:   id,
		MeetingLink: link,
		Provider:    m.Name(),
	}, nil
}

// NewTelehealthManager initializes the appropriate telehealth provider
func NewTelehealthManager(fallbackSecret ...[]byte) Provider {
	dailyKey := os.Getenv("DAILY_API_KEY")
	if strings.TrimSpace(dailyKey) != "" {
		return NewDailyProvider(dailyKey)
	}
	domain := os.Getenv("JITSI_DOMAIN")
	secret := strings.TrimSpace(os.Getenv("TELEHEALTH_SECRET"))
	if secret == "" && len(fallbackSecret) > 0 && len(fallbackSecret[0]) > 0 {
		secret = hex.EncodeToString(fallbackSecret[0])
	}
	return NewJitsiProvider(domain, secret)
}

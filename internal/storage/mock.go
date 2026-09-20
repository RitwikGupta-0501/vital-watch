package storage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MockProvider implements storage.Provider in-memory for testing
type MockProvider struct {
	mu               sync.RWMutex
	Files            map[string][]byte
	UploadURLs       map[string]string
	DownloadURLs     map[string]string
	GetFileBytesFunc func(ctx context.Context, key string) ([]byte, string, error)
}

// NewMockProvider creates a new MockProvider instance
func NewMockProvider() *MockProvider {
	return &MockProvider{
		Files:        make(map[string][]byte),
		UploadURLs:   make(map[string]string),
		DownloadURLs: make(map[string]string),
	}
}

func (m *MockProvider) GenerateUploadURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	url := fmt.Sprintf("https://mock-storage.local/upload/%s?expires=%d", key, time.Now().Add(expiry).Unix())
	m.UploadURLs[key] = url
	return url, nil
}

func (m *MockProvider) GenerateDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	url := fmt.Sprintf("https://mock-storage.local/download/%s?expires=%d", key, time.Now().Add(expiry).Unix())
	m.DownloadURLs[key] = url
	return url, nil
}

func (m *MockProvider) DeleteFile(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Files, key)
	delete(m.UploadURLs, key)
	delete(m.DownloadURLs, key)
	return nil
}

func (m *MockProvider) ObjectExists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, exists := m.Files[key]; exists {
		return true, nil
	}
	if _, exists := m.UploadURLs[key]; exists {
		return true, nil
	}
	return false, nil
}

func (m *MockProvider) GetFileBytes(ctx context.Context, key string) ([]byte, string, error) {
	if m.GetFileBytesFunc != nil {
		return m.GetFileBytesFunc(ctx, key)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.Files[key]
	if !ok {
		return nil, "", fmt.Errorf("file not found: %s", key)
	}
	if len(data) > MaxOCRFileSize {
		return nil, "", fmt.Errorf("%w: file exceeds limit of %d bytes", ErrFileTooLarge, MaxOCRFileSize)
	}
	mimeType := http.DetectContentType(data)
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	return data, mimeType, nil
}

package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrPathTraversal    = errors.New("invalid path: directory traversal attempt")
	ErrInvalidSignature = errors.New("invalid or tampered storage signature")
	ErrExpiredSignature = errors.New("storage url has expired")
	ErrIsDirectory      = errors.New("target is a directory, not a file")
)

// LocalProvider implements storage.Provider for local filesystem storage (offline dev)
// using HMAC-SHA256 URL signatures to match cloud pre-signed URL security.
type LocalProvider struct {
	baseDir       string
	baseURL       string
	signingSecret []byte
}

// NewLocalProvider creates a new LocalProvider instance
func NewLocalProvider(baseDir, baseURL string, signingSecret []byte) (*LocalProvider, error) {
	cleanDir, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local storage path: %w", err)
	}
	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to initialize local storage directory: %w", err)
	}

	cleanBaseURL := strings.TrimRight(baseURL, "/")
	return &LocalProvider{
		baseDir:       cleanDir,
		baseURL:       cleanBaseURL,
		signingSecret: signingSecret,
	}, nil
}

// resolvePath checks that the target key remains securely contained within baseDir
func (l *LocalProvider) resolvePath(key string) (string, error) {
	if strings.HasPrefix(key, "/") || strings.HasPrefix(key, "\\") || filepath.IsAbs(key) {
		return "", ErrPathTraversal
	}
	cleanKey := filepath.Clean(key)
	targetPath, err := filepath.Abs(filepath.Join(l.baseDir, cleanKey))
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(l.baseDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", ErrPathTraversal
	}

	return targetPath, nil
}

// GenerateSignature generates an HMAC-SHA256 hex string over the action, key, and expiry timestamp
func (l *LocalProvider) GenerateSignature(action, key string, expiresUnix int64) string {
	mac := hmac.New(sha256.New, l.signingSecret)
	message := fmt.Sprintf("%s:%s:%d", strings.ToUpper(action), key, expiresUnix)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature validates that the provided signature matches using constant-time comparison
func (l *LocalProvider) VerifySignature(action, key string, expiresUnix int64, providedSig string) error {
	if time.Now().Unix() > expiresUnix {
		return ErrExpiredSignature
	}
	expectedSig := l.GenerateSignature(action, key, expiresUnix)
	if !hmac.Equal([]byte(providedSig), []byte(expectedSig)) {
		return ErrInvalidSignature
	}
	return nil
}

func (l *LocalProvider) GenerateUploadURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	expires := time.Now().Add(expiry).Unix()
	sig := l.GenerateSignature("PUT", key, expires)
	return fmt.Sprintf("%s/storage/upload?key=%s&expires=%d&sig=%s", l.baseURL, url.QueryEscape(key), expires, sig), nil
}

func (l *LocalProvider) GenerateDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	expires := time.Now().Add(expiry).Unix()
	sig := l.GenerateSignature("GET", key, expires)
	return fmt.Sprintf("%s/storage/download?key=%s&expires=%d&sig=%s", l.baseURL, url.QueryEscape(key), expires, sig), nil
}

func (l *LocalProvider) DeleteFile(ctx context.Context, key string) error {
	filePath, err := l.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SaveLocalFile writes data to disk safely within baseDir
func (l *LocalProvider) SaveLocalFile(key string, data []byte) error {
	destPath, err := l.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0644)
}

// GetLocalFilePath verifies the path and ensures the file exists and is not a directory
func (l *LocalProvider) GetLocalFilePath(key string) (string, error) {
	destPath, err := l.resolvePath(key)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(destPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", ErrIsDirectory
	}
	return destPath, nil
}

// ObjectExists returns true if the key resolves to an existing file (not a directory)
func (l *LocalProvider) ObjectExists(ctx context.Context, key string) (bool, error) {
	destPath, err := l.resolvePath(key)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

func (l *LocalProvider) GetFileBytes(ctx context.Context, key string) ([]byte, string, error) {
	filePath, err := l.resolvePath(key)
	if err != nil {
		return nil, "", err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, "", err
	}
	if info.Size() > MaxOCRFileSize {
		return nil, "", fmt.Errorf("%w: size %d exceeds limit of %d bytes", ErrFileTooLarge, info.Size(), MaxOCRFileSize)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxOCRFileSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxOCRFileSize {
		return nil, "", fmt.Errorf("%w: file exceeds limit of %d bytes", ErrFileTooLarge, MaxOCRFileSize)
	}
	mimeType := http.DetectContentType(data)
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	return data, mimeType, nil
}

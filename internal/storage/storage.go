package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidFileType    = errors.New("invalid file type: only PDF and image files (.png, .jpg, .jpeg) are permitted")
	ErrInvalidContentType = errors.New("invalid content type: only application/pdf, image/png, image/jpeg are permitted")
	ErrInvalidMagicBytes  = errors.New("file content does not match allowed types: expected PDF or image (PNG/JPEG)")
)

// Allowed file extensions and MIME types for prescriptions
var allowedExtensions = map[string]bool{
	".pdf":  true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
}

var allowedMIMETypes = map[string]bool{
	"application/pdf": true,
	"image/png":       true,
	"image/jpeg":      true,
}

// ValidatePrescriptionFileType checks if the filename extension and MIME type are permitted
func ValidatePrescriptionFileType(filename, contentType string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	if !allowedExtensions[ext] {
		return ErrInvalidFileType
	}

	cleanContentType := strings.ToLower(strings.TrimSpace(contentType))
	if !allowedMIMETypes[cleanContentType] {
		return ErrInvalidContentType
	}

	return nil
}

// IsValidPrescriptionKey validates that the storage key strictly conforms to:
// "prescription-<patient_id>-<uuid>.<ext>" and contains no path separators (case-insensitive extension)
func IsValidPrescriptionKey(patientID uuid.UUID, key string) bool {
	if filepath.Base(key) != key || strings.Contains(key, "/") || strings.Contains(key, "\\") {
		return false
	}
	expectedPrefix := fmt.Sprintf("prescription-%s-", patientID.String())
	if !strings.HasPrefix(key, expectedPrefix) {
		return false
	}
	rest := strings.TrimPrefix(key, expectedPrefix)
	origExt := filepath.Ext(rest)
	if !allowedExtensions[strings.ToLower(origExt)] {
		return false
	}
	rawUUID := strings.TrimSuffix(rest, origExt)
	fileUUID, err := uuid.Parse(rawUUID)
	if err != nil || fileUUID == uuid.Nil {
		return false
	}
	return true
}

// ValidateMagicBytes inspects the leading bytes of payload using http.DetectContentType
func ValidateMagicBytes(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty file payload")
	}
	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	detected := http.DetectContentType(data[:sniffLen])
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(detected, ";")[0]))
	if !allowedMIMETypes[mimeType] {
		return mimeType, ErrInvalidMagicBytes
	}
	return mimeType, nil
}

// Provider defines the storage interface for file operations
type Provider interface {
	// GenerateUploadURL creates a pre-signed URL allowing a client to upload a file directly
	GenerateUploadURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error)

	// GenerateDownloadURL creates a short-lived pre-signed URL allowing a client to download a file
	GenerateDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// DeleteFile removes a file from storage
	DeleteFile(ctx context.Context, key string) error

	// ObjectExists checks whether an object exists in the storage provider
	ObjectExists(ctx context.Context, key string) (bool, error)
}

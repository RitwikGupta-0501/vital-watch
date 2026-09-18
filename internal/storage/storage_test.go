package storage

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidatePrescriptionFileType(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		contentType string
		wantErr     bool
	}{
		{"Valid PDF", "prescription.pdf", "application/pdf", false},
		{"Valid PNG", "scan.png", "image/png", false},
		{"Valid JPG", "photo.jpg", "image/jpeg", false},
		{"Valid JPEG uppercase", "PHOTO.JPEG", "image/jpeg", false},
		{"Invalid extension .exe", "malware.exe", "application/pdf", true},
		{"Invalid extension .html (XSS risk)", "page.html", "text/html", true},
		{"Invalid extension .sh", "script.sh", "application/x-sh", true},
		{"Mismatched/invalid MIME", "doc.pdf", "text/plain", true},
		{"Empty filename", "", "application/pdf", true},
		{"Empty content type", "scan.png", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePrescriptionFileType(tt.filename, tt.contentType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePrescriptionFileType(%s, %s) error = %v, wantErr %v", tt.filename, tt.contentType, err, tt.wantErr)
			}
		})
	}
}

func TestIsValidPrescriptionKey(t *testing.T) {
	patientID := uuid.New()
	fileID := uuid.New()
	otherPatientID := uuid.New()

	validKey := "prescription-" + patientID.String() + "-" + fileID.String() + ".pdf"

	if !IsValidPrescriptionKey(patientID, validKey) {
		t.Fatalf("expected valid key to pass")
	}

	// Uppercase extension (must pass)
	upperKey := "prescription-" + patientID.String() + "-" + fileID.String() + ".PDF"
	if !IsValidPrescriptionKey(patientID, upperKey) {
		t.Fatalf("expected uppercase .PDF key to pass")
	}

	// Wrong patient ID
	if IsValidPrescriptionKey(otherPatientID, validKey) {
		t.Fatalf("expected key with different patient ID to be rejected")
	}

	// Path traversal attempt
	traversalKey := "../" + validKey
	if IsValidPrescriptionKey(patientID, traversalKey) {
		t.Fatalf("expected path traversal key to be rejected")
	}

	// Nested slash attempt
	slashKey := "prescriptions/" + validKey
	if IsValidPrescriptionKey(patientID, slashKey) {
		t.Fatalf("expected slash key to be rejected")
	}

	// Disallowed extension
	exeKey := "prescription-" + patientID.String() + "-" + fileID.String() + ".exe"
	if IsValidPrescriptionKey(patientID, exeKey) {
		t.Fatalf("expected .exe key to be rejected")
	}

	// Malformed UUID
	malformedUUIDKey := "prescription-" + patientID.String() + "-not-a-uuid.pdf"
	if IsValidPrescriptionKey(patientID, malformedUUIDKey) {
		t.Fatalf("expected malformed UUID key to be rejected")
	}
}

func TestValidateMagicBytes(t *testing.T) {
	pdfBytes := []byte("%PDF-1.5 %\xe2\xe3\xcf\xd3\n1 0 obj")
	if mime, err := ValidateMagicBytes(pdfBytes); err != nil || mime != "application/pdf" {
		t.Fatalf("expected application/pdf, got %s, err: %v", mime, err)
	}

	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	if mime, err := ValidateMagicBytes(pngBytes); err != nil || mime != "image/png" {
		t.Fatalf("expected image/png, got %s, err: %v", mime, err)
	}

	htmlBytes := []byte("<!DOCTYPE html><html><script>alert(1)</script></html>")
	if _, err := ValidateMagicBytes(htmlBytes); err == nil {
		t.Fatalf("expected error for HTML payload, got nil")
	}

	if _, err := ValidateMagicBytes([]byte{}); err == nil {
		t.Fatalf("expected error for empty payload")
	}
}

func TestLocalProvider_Security(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vital-watch-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	secret := []byte("super-secure-local-storage-secret-key-32b")
	provider, err := NewLocalProvider(tempDir, "http://localhost:8080", secret)
	if err != nil {
		t.Fatalf("failed to create local provider: %v", err)
	}

	ctx := context.Background()
	patientID := uuid.New()
	fileID := uuid.New()
	key := "prescription-" + patientID.String() + "-" + fileID.String() + ".pdf"

	// 1. URL generation and query parsing
	uploadURL, err := provider.GenerateUploadURL(ctx, key, "application/pdf", 5*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate upload URL: %v", err)
	}

	u, err := url.Parse(uploadURL)
	if err != nil {
		t.Fatalf("failed to parse generated URL: %v", err)
	}
	q := u.Query()
	expStr := q.Get("expires")
	sig := q.Get("sig")
	if expStr == "" || sig == "" {
		t.Fatalf("missing expires or sig in upload URL")
	}
	expUnix, _ := strconv.ParseInt(expStr, 10, 64)

	// Valid PUT verification
	if err := provider.VerifySignature("PUT", key, expUnix, sig); err != nil {
		t.Fatalf("expected valid signature, got: %v", err)
	}

	// Action-binding verification (GET should be rejected for PUT signature)
	if err := provider.VerifySignature("GET", key, expUnix, sig); err == nil {
		t.Fatalf("expected signature substitution error for GET action")
	}

	// Tampered key
	if err := provider.VerifySignature("PUT", "different-key.pdf", expUnix, sig); err == nil {
		t.Fatalf("expected error for tampered key")
	}

	// Expired signature
	pastExp := time.Now().Add(-1 * time.Minute).Unix()
	pastSig := provider.GenerateSignature("PUT", key, pastExp)
	if err := provider.VerifySignature("PUT", key, pastExp, pastSig); err != ErrExpiredSignature {
		t.Fatalf("expected ErrExpiredSignature, got: %v", err)
	}

	// 2. Path traversal attack prevention
	traversalAttempts := []string{
		"../../etc/passwd",
		"../local.go",
		"/etc/shadow",
		"foo/../../etc/hosts",
	}
	for _, attackKey := range traversalAttempts {
		err := provider.SaveLocalFile(attackKey, []byte("evil content"))
		if err != ErrPathTraversal {
			t.Fatalf("expected ErrPathTraversal for key %s, got: %v", attackKey, err)
		}
		err = provider.DeleteFile(ctx, attackKey)
		if err != ErrPathTraversal {
			t.Fatalf("expected ErrPathTraversal for delete key %s, got: %v", attackKey, err)
		}
	}

	// 3. Normal file save and retrieval
	pdfData := []byte("%PDF-1.4 legitimate prescription")
	if err := provider.SaveLocalFile(key, pdfData); err != nil {
		t.Fatalf("failed to save legitimate file: %v", err)
	}

	savedPath, err := provider.GetLocalFilePath(key)
	if err != nil {
		t.Fatalf("failed to get local file path: %v", err)
	}
	if !strings.HasPrefix(savedPath, tempDir) {
		t.Fatalf("expected savedPath to be within tempDir, got: %s", savedPath)
	}

	if err := provider.DeleteFile(ctx, key); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	// Directory check
	subDir := filepath.Join(tempDir, "testsubdir")
	os.MkdirAll(subDir, 0755)
	if _, err := provider.GetLocalFilePath("testsubdir"); err != ErrIsDirectory {
		t.Fatalf("expected ErrIsDirectory for directory key, got: %v", err)
	}
}

func TestMockProvider(t *testing.T) {
	mock := NewMockProvider()
	ctx := context.Background()
	key := "prescriptions/test-key.pdf"

	uploadURL, err := mock.GenerateUploadURL(ctx, key, "application/pdf", 5*time.Minute)
	if err != nil || uploadURL == "" {
		t.Fatalf("expected valid upload URL from mock, got %s, err: %v", uploadURL, err)
	}

	downloadURL, err := mock.GenerateDownloadURL(ctx, key, 5*time.Minute)
	if err != nil || downloadURL == "" {
		t.Fatalf("expected valid download URL from mock, got %s, err: %v", downloadURL, err)
	}

	if err := mock.DeleteFile(ctx, key); err != nil {
		t.Fatalf("failed to delete mock file: %v", err)
	}

	if _, ok := mock.UploadURLs[key]; ok {
		t.Fatalf("expected upload URL to be removed from mock")
	}
}

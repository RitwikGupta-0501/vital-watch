package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func TestGetPrescriptionUploadURL(t *testing.T) {
	mockStorage := storage.NewMockProvider()
	h := &Handler{
		Storage: mockStorage,
	}

	r := gin.New()
	r.POST("/upload-url", h.GetPrescriptionUploadURL)

	patientID := uuid.New()

	// 1. Valid PDF upload request -> 200 OK
	validBody, _ := json.Marshal(map[string]interface{}{
		"patient_id":   patientID.String(),
		"filename":     "prescription_scan.pdf",
		"content_type": "application/pdf",
	})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/upload-url", bytes.NewReader(validBody))
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid PDF, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	var resp1 map[string]interface{}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp1["upload_url"] == "" || resp1["file_key"] == "" {
		t.Fatalf("expected non-empty upload_url and file_key, got %v", resp1)
	}

	// 2. Prohibited extension (.exe) -> 400 Bad Request
	malwareBody, _ := json.Marshal(map[string]interface{}{
		"patient_id":   patientID.String(),
		"filename":     "script.exe",
		"content_type": "application/pdf",
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/upload-url", bytes.NewReader(malwareBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for .exe file, got %d", w2.Code)
	}

	// 3. Prohibited HTML file (Stored XSS) -> 400 Bad Request
	xssBody, _ := json.Marshal(map[string]interface{}{
		"patient_id":   patientID.String(),
		"filename":     "attack.html",
		"content_type": "text/html",
	})
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/upload-url", bytes.NewReader(xssBody))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for .html file, got %d", w3.Code)
	}

	// 4. Missing/nil patient_id -> 400 Bad Request
	emptyPatientBody, _ := json.Marshal(map[string]interface{}{
		"patient_id":   uuid.Nil.String(),
		"filename":     "prescription.png",
		"content_type": "image/png",
	})
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodPost, "/upload-url", bytes.NewReader(emptyPatientBody))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)

	if w4.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for nil patient_id, got %d", w4.Code)
	}
}

func TestCreatePrescription_KeyValidationAndExistence(t *testing.T) {
	mockStorage := storage.NewMockProvider()
	h := &Handler{
		Storage: mockStorage,
	}

	doctorID := uuid.New()
	patientID := uuid.New()
	victimID := uuid.New()

	r := gin.New()
	r.POST("/prescriptions", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.CreatePrescription(c)
	})

	// 1. Cross-patient filename IDOR attack attempt -> 400 Bad Request
	rogueKey := "prescription-" + victimID.String() + "-" + uuid.New().String() + ".pdf"
	idorBody, _ := json.Marshal(map[string]interface{}{
		"patient_id": patientID.String(),
		"medication": "Amoxicillin 500mg",
		"notes":      "Take after food",
		"file_name":  rogueKey,
	})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(idorBody))
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for cross-patient file key IDOR, got %d", w1.Code)
	}

	// 2. Directory traversal filename attempt -> 400 Bad Request
	traversalKey := "../../etc/passwd"
	travBody, _ := json.Marshal(map[string]interface{}{
		"patient_id": patientID.String(),
		"medication": "Amoxicillin 500mg",
		"notes":      "Take after food",
		"file_name":  traversalKey,
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(travBody))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for directory traversal file_name, got %d", w2.Code)
	}

	// 3. Phantom file attempt (file key formatted correctly, but file does not exist in storage) -> 400 Bad Request
	validUnuploadedKey := "prescription-" + patientID.String() + "-" + uuid.New().String() + ".pdf"
	phantomBody, _ := json.Marshal(map[string]interface{}{
		"patient_id": patientID.String(),
		"medication": "Amoxicillin 500mg",
		"notes":      "Take after food",
		"file_name":  validUnuploadedKey,
	})
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(phantomBody))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for phantom unuploaded file, got %d", w3.Code)
	}
}

func TestLocalStorageHTTPHandlers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vital-watch-api-storage-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	secret := []byte("api-test-local-storage-secret-key-32b")
	localProv, err := storage.NewLocalProvider(tempDir, "http://localhost:8080", secret)
	if err != nil {
		t.Fatalf("failed to create local provider: %v", err)
	}

	h := &Handler{
		Storage: localProv,
	}

	r := gin.New()
	r.PUT("/storage/upload", h.HandleLocalStorageUpload)
	r.GET("/storage/download", h.HandleLocalStorageDownload)

	key := "prescription-" + uuid.New().String() + "-" + uuid.New().String() + ".pdf"
	expires := time.Now().Add(5 * time.Minute).Unix()
	sig := localProv.GenerateSignature("PUT", key, expires)

	// 1. Valid upload -> 200 OK
	pdfPayload := []byte("%PDF-1.4 sample valid prescription content")
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/storage/upload?key=%s&expires=%d&sig=%s", url.QueryEscape(key), expires, sig), bytes.NewReader(pdfPayload))
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid upload, got %d, body: %s", w1.Code, w1.Body.String())
	}

	// 2. Download with valid signature -> 200 OK + Content-Disposition: attachment
	dlSig := localProv.GenerateSignature("GET", key, expires)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/storage/download?key=%s&expires=%d&sig=%s", url.QueryEscape(key), expires, dlSig), nil)
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid download, got %d", w2.Code)
	}
	if w2.Header().Get("Content-Disposition") == "" {
		t.Fatalf("expected Content-Disposition header")
	}
	if w2.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options: nosniff")
	}

	// 3. Upload with invalid signature -> 403 Forbidden
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/storage/upload?key=%s&expires=%d&sig=invalidsig", url.QueryEscape(key), expires), bytes.NewReader(pdfPayload))
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for tampered signature, got %d", w3.Code)
	}

	// 4. Upload with bad magic bytes (HTML XSS payload) -> 400 Bad Request
	htmlPayload := []byte("<!DOCTYPE html><html><script>alert(1)</script></html>")
	htmlKey := "prescription-" + uuid.New().String() + "-" + uuid.New().String() + ".pdf"
	htmlSig := localProv.GenerateSignature("PUT", htmlKey, expires)
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/storage/upload?key=%s&expires=%d&sig=%s", url.QueryEscape(htmlKey), expires, htmlSig), bytes.NewReader(htmlPayload))
	r.ServeHTTP(w4, req4)

	if w4.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for html magic bytes payload, got %d", w4.Code)
	}
}

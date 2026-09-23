package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLoggerContext(t *testing.T) {
	ctx := context.Background()
	if GetRequestID(ctx) != "" {
		t.Error("expected empty request ID on empty context")
	}

	testID := "test-request-id-1234"
	ctx = WithRequestID(ctx, testID)

	if GetRequestID(ctx) != testID {
		t.Errorf("expected request ID %s, got %s", testID, GetRequestID(ctx))
	}

	log := FromContext(ctx)
	if log == nil {
		t.Fatal("expected non-nil logger from context")
	}
}

func TestContextHandler_InjectsRequestID(t *testing.T) {
	var buf bytes.Buffer
	testLogger := InitLoggerWithWriter(&buf)

	testID := "req-abc-987"
	ctx := WithRequestID(context.Background(), testID)

	testLogger.InfoContext(ctx, "testing context injection", "user_id", "user-42")

	output := buf.String()
	var logMap map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logMap); err != nil {
		t.Fatalf("failed to parse JSON log output: %v; raw: %s", err, output)
	}

	if logMap["msg"] != "testing context injection" {
		t.Errorf("expected msg %q, got %q", "testing context injection", logMap["msg"])
	}
	if logMap["request_id"] != testID {
		t.Errorf("expected request_id %q, got %q", testID, logMap["request_id"])
	}
	if logMap["user_id"] != "user-42" {
		t.Errorf("expected user_id %q, got %q", "user-42", logMap["user_id"])
	}
}

func TestContextHandler_NoRequestID(t *testing.T) {
	var buf bytes.Buffer
	testLogger := InitLoggerWithWriter(&buf)

	testLogger.InfoContext(context.Background(), "no request id present")

	output := buf.String()
	var logMap map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logMap); err != nil {
		t.Fatalf("failed to parse JSON log output: %v; raw: %s", err, output)
	}

	if _, exists := logMap["request_id"]; exists {
		t.Errorf("request_id should not exist in log output when not in context: %+v", logMap)
	}
}

func TestInitLoggerWithWriter_TextFormat(t *testing.T) {
	os.Setenv("LOG_FORMAT", "text")
	defer os.Unsetenv("LOG_FORMAT")

	var buf bytes.Buffer
	testLogger := InitLoggerWithWriter(&buf)

	ctx := WithRequestID(context.Background(), "text-req-123")
	testLogger.InfoContext(ctx, "text log message")

	output := buf.String()
	if !strings.Contains(output, "request_id=text-req-123") {
		t.Errorf("expected output to contain request_id=text-req-123, got: %s", output)
	}
	if !strings.Contains(output, "text log message") && !strings.Contains(output, "text log message") {
		t.Errorf("expected output to contain msg text log message, got: %s", output)
	}
}

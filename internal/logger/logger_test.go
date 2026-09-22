package logger

import (
	"context"
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

package main

import (
	"testing"

	"github.com/RitwikGupta-0501/vital-watch/internal/api"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func TestSetupRouter_NoRouteConflicts(t *testing.T) {
	h := &api.Handler{
		Storage:   storage.NewMockProvider(),
		JWTSecret: []byte("test-secret"),
	}

	// Verify both local and S3 storage type configurations initialize without panicking
	rLocal := setupRouter(h, "local", []byte("test-secret"))
	if rLocal == nil {
		t.Fatal("expected non-nil gin engine for local storage")
	}

	rS3 := setupRouter(h, "s3", []byte("test-secret"))
	if rS3 == nil {
		t.Fatal("expected non-nil gin engine for s3 storage")
	}
}

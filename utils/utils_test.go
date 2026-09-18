package utils

import (
	"testing"
)

func TestHashPasswordAndCheck(t *testing.T) {
	password := "SecretPass123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("expected no error from HashPassword, got: %v", err)
	}

	if hash == "" || hash == password {
		t.Fatalf("expected valid non-empty bcrypt hash, got: %s", hash)
	}

	// Verify correct password matches
	if !CheckPasswordHash(password, hash) {
		t.Fatalf("expected CheckPasswordHash to return true for correct password")
	}

	// Verify incorrect password fails
	if CheckPasswordHash("WrongPassword", hash) {
		t.Fatalf("expected CheckPasswordHash to return false for wrong password")
	}
}

package utils

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		expectedErr error
	}{
		{
			name:        "Valid complex password",
			password:    "ValidPass123!",
			expectedErr: nil,
		},
		{
			name:        "Valid another strong password",
			password:    "Dr#JohnDoe2026$",
			expectedErr: nil,
		},
		{
			name:        "Too short (< 8 chars)",
			password:    "Ab1!foo",
			expectedErr: ErrPasswordTooShort,
		},
		{
			name:        "Too long (> 72 bytes)",
			password:    strings.Repeat("A", 70) + "1!a",
			expectedErr: ErrPasswordTooLong,
		},
		{
			name:        "Missing uppercase",
			password:    "password123!",
			expectedErr: ErrPasswordCommonWeak, // Also flagged as common weak password
		},
		{
			name:        "Missing uppercase non-common",
			password:    "uncommonpass123!",
			expectedErr: ErrPasswordNoUpper,
		},
		{
			name:        "Missing lowercase",
			password:    "UNCOMMONPASS123!",
			expectedErr: ErrPasswordNoLower,
		},
		{
			name:        "Missing digit",
			password:    "UncommonPassword!",
			expectedErr: ErrPasswordNoNumber,
		},
		{
			name:        "Missing special character",
			password:    "UncommonPassword123",
			expectedErr: ErrPasswordNoSpecial,
		},
		{
			name:        "Common weak password (case insensitive)",
			password:    "Password123!",
			expectedErr: ErrPasswordCommonWeak,
		},
		{
			name:        "Common admin weak password",
			password:    "Admin1234!",
			expectedErr: ErrPasswordCommonWeak,
		},
		{
			name:        "Common healthcare weak password",
			password:    "Healthcare1!",
			expectedErr: ErrPasswordCommonWeak,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.expectedErr == nil {
				if err != nil {
					t.Fatalf("expected nil error for %q, got: %v", tt.password, err)
				}
			} else {
				if !errors.Is(err, tt.expectedErr) {
					t.Fatalf("expected %v for %q, got: %v", tt.expectedErr, tt.password, err)
				}
			}
		})
	}
}

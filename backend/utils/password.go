package utils

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrPasswordTooShort     = errors.New("password must be at least 8 characters long")
	ErrPasswordTooLong      = errors.New("password cannot exceed 72 bytes")
	ErrPasswordNoUpper      = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLower      = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoNumber     = errors.New("password must contain at least one digit")
	ErrPasswordNoSpecial    = errors.New("password must contain at least one special character")
	ErrPasswordCommonWeak   = errors.New("password is too common or easily guessable")
)

// commonWeakPasswords contains commonly used weak passwords that should be rejected (NIST SP 800-63B).
var commonWeakPasswords = map[string]struct{}{
	"password":       {},
	"password123":    {},
	"password123!":   {},
	"admin123":       {},
	"admin1234":      {},
	"admin1234!":     {},
	"administrator":  {},
	"healthcare1":    {},
	"healthcare1!":   {},
	"vitalwatch1":    {},
	"vitalwatch123":  {},
	"vitalwatch123!": {},
	"welcome1":       {},
	"welcome123":     {},
	"welcome123!":    {},
	"letmein123!":    {},
	"changeme123!":   {},
}

// ValidatePassword validates password complexity according to HIPAA § 164.312 and NIST SP 800-63B.
// Rules:
// - Length between 8 and 72 bytes (bcrypt max length)
// - At least one uppercase character [A-Z]
// - At least one lowercase character [a-z]
// - At least one digit [0-9]
// - At least one special/punctuation character
// - Not present in common weak passwords dictionary
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 72 {
		return ErrPasswordTooLong
	}

	// Check common weak passwords (case-insensitive)
	lowerPass := strings.ToLower(strings.TrimSpace(password))
	if _, common := commonWeakPasswords[lowerPass]; common {
		return ErrPasswordCommonWeak
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return ErrPasswordNoUpper
	}
	if !hasLower {
		return ErrPasswordNoLower
	}
	if !hasNumber {
		return ErrPasswordNoNumber
	}
	if !hasSpecial {
		return ErrPasswordNoSpecial
	}

	return nil
}

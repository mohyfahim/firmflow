package security

import (
	"fmt"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

var (
	upperRe  = regexp.MustCompile(`[A-Z]`)
	lowerRe  = regexp.MustCompile(`[a-z]`)
	digitRe  = regexp.MustCompile(`[0-9]`)
	symbolRe = regexp.MustCompile(`[^A-Za-z0-9]`)
)

func ValidatePasswordStrength(password string, minLen int) error {
	if len(password) < minLen {
		return fmt.Errorf("password must be at least %d characters", minLen)
	}
	if !upperRe.MatchString(password) || !lowerRe.MatchString(password) || !digitRe.MatchString(password) || !symbolRe.MatchString(password) {
		return fmt.Errorf("password must include uppercase, lowercase, digit, and symbol")
	}
	return nil
}

func HashPassword(password string, cost int) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ComparePassword(hash, raw string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(raw))
}

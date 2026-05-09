package security

import "testing"

func TestValidatePasswordStrength(t *testing.T) {
	if err := ValidatePasswordStrength("weak", 12); err == nil {
		t.Fatal("expected weak password error")
	}
	if err := ValidatePasswordStrength("Str0ng!Password", 12); err != nil {
		t.Fatalf("expected strong password to pass: %v", err)
	}
}

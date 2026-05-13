package service

import (
	"testing"
)

func TestParseSemverParts_valid(t *testing.T) {
	sp := parseSemverParts("v1.2.3")
	if sp.Major == nil || *sp.Major != 1 || sp.Minor == nil || *sp.Minor != 2 || sp.Patch == nil || *sp.Patch != 3 {
		t.Fatalf("semver parts: %+v", sp)
	}
	if sp.Prerelease != "" {
		t.Fatalf("prerelease: %q", sp.Prerelease)
	}
}

func TestParseSemverParts_prerelease(t *testing.T) {
	sp := parseSemverParts("1.0.0-rc.1")
	if sp.Prerelease != "rc.1" {
		t.Fatalf("prerelease: %q", sp.Prerelease)
	}
}

func TestParseSemverParts_nonSemver(t *testing.T) {
	sp := parseSemverParts("  my-custom-tag  ")
	if sp.Major != nil || sp.Minor != nil || sp.Patch != nil {
		t.Fatalf("expected no semver tuple, got %+v", sp)
	}
}

func TestNormalizeVersion(t *testing.T) {
	if got := normalizeVersion("  1.0.0 "); got != "1.0.0" {
		t.Fatalf("got %q", got)
	}
}

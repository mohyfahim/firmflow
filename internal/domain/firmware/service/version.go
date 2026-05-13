package service

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

func normalizeVersion(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

type semverParts struct {
	Major, Minor, Patch *int
	Prerelease          string
}

func parseSemverParts(raw string) semverParts {
	v, err := semver.NewVersion(strings.TrimSpace(raw))
	if err != nil {
		return semverParts{}
	}
	mj := int(v.Major())
	mn := int(v.Minor())
	pt := int(v.Patch())
	return semverParts{
		Major:      &mj,
		Minor:      &mn,
		Patch:      &pt,
		Prerelease: v.Prerelease(),
	}
}

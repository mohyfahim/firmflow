package service

import (
	"sort"

	"github.com/google/uuid"
)

func sortUUIDs(ids []uuid.UUID) []uuid.UUID {
	cp := append([]uuid.UUID(nil), ids...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].String() < cp[j].String() })
	return cp
}

// takeStablePercentage returns the first ceil(len*percent/100) IDs from sortedUUIDs (sorted ascending).
// Deterministic: same project scope + percent + ordering yields the same subset across runs.
func takeStablePercentage(sortedUUIDs []uuid.UUID, percent int) []uuid.UUID {
	if len(sortedUUIDs) == 0 {
		return nil
	}
	if percent < 1 {
		percent = 1
	}
	if percent > 100 {
		percent = 100
	}
	k := (len(sortedUUIDs)*percent + 99) / 100
	if k < 1 {
		k = 1
	}
	if k > len(sortedUUIDs) {
		k = len(sortedUUIDs)
	}
	out := make([]uuid.UUID, k)
	copy(out, sortedUUIDs[:k])
	return out
}

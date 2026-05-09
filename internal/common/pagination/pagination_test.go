package pagination

import "testing"

func TestBuildMeta(t *testing.T) {
	meta := BuildMeta(2, 20, 45)
	if meta.TotalPages != 3 {
		t.Fatalf("expected total pages to be 3, got %d", meta.TotalPages)
	}
	if meta.Page != 2 {
		t.Fatalf("expected page to be 2, got %d", meta.Page)
	}
}

package pagination

import "testing"

func TestNormalizeAppliesDefaults(t *testing.T) {
	r := &PageRequest{}
	page, pageSize := r.Normalize()
	if page != DefaultPage || pageSize != DefaultPageSize {
		t.Fatalf("Normalize() = (%d, %d), want (%d, %d)", page, pageSize, DefaultPage, DefaultPageSize)
	}
}

func TestNormalizeRejectsNonPositiveValues(t *testing.T) {
	r := &PageRequest{Page: -1, PageSize: 0}
	page, pageSize := r.Normalize()
	if page != DefaultPage || pageSize != DefaultPageSize {
		t.Fatalf("Normalize() = (%d, %d), want (%d, %d)", page, pageSize, DefaultPage, DefaultPageSize)
	}
}

// TestNormalizeCapsPageSize 验证 page_size 会被限制在 MaxPageSize。
func TestNormalizeCapsPageSize(t *testing.T) {
	r := &PageRequest{Page: 1, PageSize: 1_000_000}
	_, pageSize := r.Normalize()
	if pageSize != MaxPageSize {
		t.Fatalf("Normalize() pageSize = %d, want capped at %d", pageSize, MaxPageSize)
	}
}

func TestOffsetAndLimit(t *testing.T) {
	r := &PageRequest{Page: 3, PageSize: 10}
	if got := r.Offset(); got != 20 {
		t.Fatalf("Offset() = %d, want 20", got)
	}
	if got := r.Limit(); got != 10 {
		t.Fatalf("Limit() = %d, want 10", got)
	}
}

func TestFromRequestComputesTotalPages(t *testing.T) {
	req := &PageRequest{Page: 1, PageSize: 10}
	resp := FromRequest(req, []int{1, 2, 3}, 25)
	if resp.TotalPages != 3 {
		t.Fatalf("TotalPages = %d, want 3", resp.TotalPages)
	}
}

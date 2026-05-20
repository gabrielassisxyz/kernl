package ids

import (
	"sort"
	"testing"
)

func TestUUIDv7Monotonic(t *testing.T) {
	const n = 1000
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = New()
	}
	// Assert each ID is string-greater-than the previous one.
	for i := 1; i < n; i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("IDs not monotonic at index %d: %s <= %s", i, ids[i], ids[i-1])
		}
	}
	// Verify sort.Strings does not reorder.
	sorted := make([]string, n)
	copy(sorted, ids)
	sort.Strings(sorted)
	for i := 0; i < n; i++ {
		if sorted[i] != ids[i] {
			t.Fatalf("sort.Strings changed order at index %d: expected %s, got %s", i, ids[i], sorted[i])
		}
	}
}

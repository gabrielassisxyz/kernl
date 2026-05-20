package ids

import (
	"sort"
	"testing"
	"time"
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

func TestUUIDv7SortableByCreationTime(t *testing.T) {
	// Generate 5 UUIDs with small sleep gaps.
	const n = 5
	uuids := make([]string, n)
	for i := 0; i < n; i++ {
		uuids[i] = New()
		time.Sleep(2 * time.Millisecond)
	}

	// Verify they are already sorted by generation order.
	for i := 1; i < n; i++ {
		if uuids[i] <= uuids[i-1] {
			t.Fatalf("UUID %d is not greater than previous: %s <= %s", i, uuids[i], uuids[i-1])
		}
	}

	// Verify sort.Strings does not change order (creation order == string order).
	sorted := make([]string, n)
	copy(sorted, uuids)
	sort.Strings(sorted)
	for i := 0; i < n; i++ {
		if sorted[i] != uuids[i] {
			t.Fatalf("sort.Strings changed order at index %d: expected %s, got %s", i, uuids[i], sorted[i])
		}
	}
}

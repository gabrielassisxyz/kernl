package backend

import (
	"fmt"
	"testing"
	"time"
)

func stubBead(id string) Bead {
	return Bead{ID: id, Title: "stub", Type: "task", State: "open", Priority: 2}
}

func okBeads(beads []Bead) BackendResult[[]Bead] {
	return OkResult(beads)
}

func failBeads(msg string) BackendResult[[]Bead] {
	return ErrResult[[]Bead](NewBackendError(ErrorCodeInternal, msg))
}

func TestIsSuppressibleError_MatchesLockRelatedMessages(t *testing.T) {
	cases := []struct {
		msg    string
		expect bool
	}{
		{"database is locked", true},
		{"could not obtain lock on table", true},
		{"Timed out waiting for bd repo lock for /repo after 15000ms", true},
		{"bd command timed out after 15000ms", true},
		{"Error: EACCES permission denied", true},
		{"resource busy", true},
		{"Failed to parse bd list output", false},
		{"bd list failed", false},
		{"unknown command", false},
	}
	for _, tc := range cases {
		got := IsSuppressibleError(tc.msg)
		if got != tc.expect {
			t.Errorf("IsSuppressibleError(%q) = %v, want %v", tc.msg, got, tc.expect)
		}
	}
}

func TestErrorSuppression_PassesThroughSuccessfulResults(t *testing.T) {
	cache := newTestCache()
	beads := []Bead{stubBead("b-1")}
	result := okBeads(beads)
	out := cache.WithErrorSuppression("listBeads", result, nil, "", "")

	if !out.OK {
		t.Error("expected OK result")
	}
	if len(*out.Data) != 1 || (*out.Data)[0].ID != "b-1" {
		t.Errorf("unexpected data: %+v", out.Data)
	}
}

func TestErrorSuppression_ReturnsDegradedWhenNoCacheForLockError(t *testing.T) {
	cache := newTestCache()
	result := failBeads("database is locked")
	out := cache.WithErrorSuppression("listBeads", result, nil, "", "")

	if out.OK {
		t.Error("expected error result")
	}
	if out.Error.Message != DegradedErrorMessage {
		t.Errorf("expected degraded message, got: %s", out.Error.Message)
	}
	if !out.Error.Retryable {
		t.Error("expected retryable=true")
	}
}

func TestErrorSuppression_ReturnsCachedDataOnFirstLockFailure(t *testing.T) {
	cache := newTestCache()
	beads := []Bead{stubBead("b-1")}
	cache.WithErrorSuppression("listBeads", okBeads(beads), nil, "", "")

	out := cache.WithErrorSuppression("listBeads", failBeads("database locked"), nil, "", "")
	if !out.OK {
		t.Error("expected OK result from cache")
	}
	if len(*out.Data) != 1 || (*out.Data)[0].ID != "b-1" {
		t.Errorf("unexpected cached data: %+v", out.Data)
	}
}

func TestErrorSuppression_KeepsReturningCachedDataWithinWindow(t *testing.T) {
	cache := newTestCache()
	beads := []Bead{stubBead("b-1")}
	cache.WithErrorSuppression("listBeads", okBeads(beads), nil, "", "")
	cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")

	out := cache.WithErrorSuppression("listBeads", failBeads("locked again"), nil, "", "")
	if !out.OK {
		t.Error("expected cached OK result")
	}
}

func TestErrorSuppression_ReturnsDegradedAfterWindowExpires(t *testing.T) {
	cache := newTestCache()
	now := time.Now()
	cache.nowFunc = func() time.Time { return now }

	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "")
	cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")

	internals := cache.Internals()
	failKeys := make([]string, 0)
	for k := range internals.FailureState() {
		failKeys = append(failKeys, k)
	}
	if len(failKeys) == 0 {
		t.Fatal("expected failure state entry")
	}

	now = now.Add(3 * time.Minute)
	cache.nowFunc = func() time.Time { return now }

	out := cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")
	if out.OK {
		t.Error("expected error after window expired")
	}
	if out.Error.Message != DegradedErrorMessage {
		t.Errorf("expected degraded message, got: %s", out.Error.Message)
	}
}

func TestErrorSuppression_ClearsFailureOnRecovery(t *testing.T) {
	cache := newTestCache()
	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "")
	cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")

	internals := cache.Internals()
	if len(internals.FailureState()) != 1 {
		t.Error("expected 1 failure state entry")
	}

	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "")
	if len(internals.FailureState()) != 0 {
		t.Error("expected failure state cleared on recovery")
	}
}

func TestErrorSuppression_SeparateCacheForDifferentSignatures(t *testing.T) {
	cache := newTestCache()
	beatA := stubBead("a")
	beatB := stubBead("b")

	cache.WithErrorSuppression("listBeads", okBeads([]Bead{beatA}), nil, "/repo-a", "")
	cache.WithErrorSuppression("listBeads", okBeads([]Bead{beatB}), nil, "/repo-b", "")

	outA := cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "/repo-a", "")
	if !outA.OK || (*outA.Data)[0].ID != "a" {
		t.Errorf("expected cached repo-a data")
	}
}

func TestErrorSuppression_DistinguishesByQuery(t *testing.T) {
	cache := newTestCache()
	cache.WithErrorSuppression("searchBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "alpha")
	cache.WithErrorSuppression("searchBeads", okBeads([]Bead{}), nil, "", "beta")

	out := cache.WithErrorSuppression("searchBeads", failBeads("locked"), nil, "", "alpha")
	if !out.OK || len(*out.Data) != 1 {
		t.Error("expected cached result for query=alpha")
	}

	outBeta := cache.WithErrorSuppression("searchBeads", failBeads("locked"), nil, "", "beta")
	if !outBeta.OK || len(*outBeta.Data) != 0 {
		t.Error("expected cached empty result for query=beta")
	}
}

func TestErrorSuppression_DoesNotSuppressNonLockErrors(t *testing.T) {
	cache := newTestCache()
	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "")

	out := cache.WithErrorSuppression("listBeads", failBeads("Failed to parse bd list output"), nil, "", "")
	if out.OK {
		t.Error("expected error for non-suppressible error")
	}
	if out.Error.Message != "Failed to parse bd list output" {
		t.Errorf("expected original error message, got: %s", out.Error.Message)
	}
}

func TestErrorSuppression_DoesNotSuppressGenericFailures(t *testing.T) {
	cache := newTestCache()
	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "")

	out := cache.WithErrorSuppression("listBeads", failBeads("bd list failed"), nil, "", "")
	if out.OK {
		t.Error("expected error for non-suppressible error")
	}
	if out.Error.Message != "bd list failed" {
		t.Errorf("expected original error message, got: %s", out.Error.Message)
	}
}

func TestErrorSuppression_EvictsOldestWhenOverMaxEntries(t *testing.T) {
	cache := newTestCache()
	for i := 0; i <= MaxCacheEntries; i++ {
		beads := []Bead{stubBead("b-1")}
		cache.WithErrorSuppression("listBeads", okBeads(beads), nil, fmt.Sprintf("/repo-%d", i), "")
	}
	internals := cache.Internals()
	rc := internals.ResultCache()
	if len(rc) > MaxCacheEntries {
		t.Errorf("expected at most %d entries, got %d", MaxCacheEntries, len(rc))
	}
}

func TestErrorSuppression_DegradedAfterWindowWithNoCache(t *testing.T) {
	cache := newTestCache()
	now := time.Now()
	cache.nowFunc = func() time.Time { return now }

	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "")
	cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")

	internals := cache.Internals()
	failKeys := make([]string, 0)
	for k := range internals.FailureState() {
		failKeys = append(failKeys, k)
	}
	cacheKeys := make([]string, 0)
	for k := range internals.ResultCache() {
		cacheKeys = append(cacheKeys, k)
	}

	now = now.Add(3 * time.Minute)
	cache.nowFunc = func() time.Time { return now }

	internals.SetResultCacheTimestamp(cacheKeys[0], now.Add(-11*time.Minute))
	internals.SetFailureFirstFailedAt(failKeys[0], now.Add(-3*time.Minute))

	out := cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")
	if out.OK {
		t.Error("expected error after window expired")
	}
	if out.Error.Message != DegradedErrorMessage {
		t.Errorf("expected degraded message, got: %s", out.Error.Message)
	}
}

func TestErrorSuppression_TTLExpiryDuringFailure(t *testing.T) {
	cache := newTestCache()
	now := time.Now()
	cache.nowFunc = func() time.Time { return now }

	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), nil, "", "")
	cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")

	internals := cache.Internals()
	cacheKeys := make([]string, 0)
	for k := range internals.ResultCache() {
		cacheKeys = append(cacheKeys, k)
	}
	failKeys := make([]string, 0)
	for k := range internals.FailureState() {
		failKeys = append(failKeys, k)
	}

	internals.SetResultCacheTimestamp(cacheKeys[0], now.Add(-11*time.Minute))
	internals.SetFailureFirstFailedAt(failKeys[0], now)

	out := cache.WithErrorSuppression("listBeads", failBeads("locked"), nil, "", "")
	if out.OK {
		t.Error("expected error when cache TTL expired")
	}
	if out.Error.Message != "locked" {
		t.Errorf("expected raw error returned on TTL expiry, got: %s", out.Error.Message)
	}
}

func TestErrorSuppression_FilterKeyOrderIndependent(t *testing.T) {
	cache := newTestCache()
	filtersA := map[string]string{"status": "open", "type": "bug"}
	cache.WithErrorSuppression("listBeads", okBeads([]Bead{stubBead("b-1")}), filtersA, "", "")

	filtersB := map[string]string{"type": "bug", "status": "open"}
	out := cache.WithErrorSuppression("listBeads", failBeads("locked"), filtersB, "", "")
	if !out.OK {
		t.Error("expected cached hit with reordered filters")
	}
}

func newTestCache() *ErrorSuppressionCache {
	cache := NewErrorSuppressionCache()
	cache.nowFunc = time.Now
	return cache
}

func TestCacheKey_Deterministic(t *testing.T) {
	cache := NewErrorSuppressionCache()

	key1 := cache.CacheKey("listBeads", map[string]string{"status": "open", "type": "bug"}, "/repo", "query")
	key2 := cache.CacheKey("listBeads", map[string]string{"type": "bug", "status": "open"}, "/repo", "query")

	if key1 != key2 {
		t.Errorf("expected same cache key for same filters in different order, got %q vs %q", key1, key2)
	}
}
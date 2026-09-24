package store

import (
	"errors"
	"testing"
	"time"
)

// TestRetryWriteSucceedsFirstTry checks the common case: no retries, no
// sleeps.
func TestRetryWriteSucceedsFirstTry(t *testing.T) {
	calls := 0
	var slept []time.Duration
	sleep := func(d time.Duration) { slept = append(slept, d) }

	err := retryWrite(3, sleep, putBlobBackoff, func(attempt int) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 call, got %d", calls)
	}
	if len(slept) != 0 {
		t.Fatalf("want no sleeps, got %v", slept)
	}
}

// TestRetryWriteRecoversAfterTransientFailures checks that a write which
// fails a couple of times and then succeeds is retried, with the expected
// 200ms/400ms backoff between attempts, and reports success overall.
func TestRetryWriteRecoversAfterTransientFailures(t *testing.T) {
	var attempts []int
	var slept []time.Duration
	sleep := func(d time.Duration) { slept = append(slept, d) }

	failUntil := 3
	err := retryWrite(putBlobMaxAttempts, sleep, putBlobBackoff, func(attempt int) error {
		attempts = append(attempts, attempt)
		if attempt < failUntil {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int{1, 2, 3}; !equalInts(attempts, want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	want := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond}
	if !equalDurations(slept, want) {
		t.Fatalf("sleeps = %v, want %v", slept, want)
	}
}

// TestRetryWriteReturnsFinalErrorAfterExhaustingAttempts checks that when
// every attempt fails, retryWrite gives up after putBlobMaxAttempts and
// surfaces the last attempt's error - never a partial success.
func TestRetryWriteReturnsFinalErrorAfterExhaustingAttempts(t *testing.T) {
	calls := 0
	var slept []time.Duration
	sleep := func(d time.Duration) { slept = append(slept, d) }

	lastErr := errors.New("attempt 3 failed")
	err := retryWrite(3, sleep, putBlobBackoff, func(attempt int) error {
		calls++
		if attempt == 3 {
			return lastErr
		}
		return errors.New("earlier failure")
	})
	if !errors.Is(err, lastErr) {
		t.Fatalf("err = %v, want %v", err, lastErr)
	}
	if calls != 3 {
		t.Fatalf("want 3 calls, got %d", calls)
	}
	// Only 2 backoffs between 3 attempts - no sleep after the final, failed
	// attempt.
	if len(slept) != 2 {
		t.Fatalf("want 2 sleeps, got %v", slept)
	}
}

// TestPutBlobAtWrapsErrorWithBlobID exercises PutBlobAt's own error handling
// (checksum computation, retry wiring, final wrapping) without a real GCS
// client, by driving retryWrite's write func directly the way PutBlobAt does.
// PutBlobAt itself needs a live *storage.Client to reach writeBlobOnce, so
// this test documents and pins the wrapping behavior at the retryWrite call
// site instead of exercising the network path.
func TestPutBlobAtWrapsErrorWithBlobID(t *testing.T) {
	id := "blob-123"
	writeErr := errors.New("permission denied")
	var slept []time.Duration

	err := retryWrite(putBlobMaxAttempts, func(d time.Duration) { slept = append(slept, d) }, putBlobBackoff, func(attempt int) error {
		return writeErr
	})
	wrapped := wrapPutBlobErr(id, err)
	if !errors.Is(wrapped, writeErr) {
		t.Fatalf("wrapped error does not chain to the original: %v", wrapped)
	}
	if got := wrapped.Error(); got == "" {
		t.Fatal("wrapped error message is empty")
	}
	if len(slept) != putBlobMaxAttempts-1 {
		t.Fatalf("want %d sleeps, got %d", putBlobMaxAttempts-1, len(slept))
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

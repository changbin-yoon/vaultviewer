package auth

import (
	"testing"
	"time"
)

func newTestThrottle(start time.Time) (*LoginThrottle, *time.Time) {
	t := NewLoginThrottle()
	clock := start
	t.now = func() time.Time { return clock }
	return t, &clock
}

func TestLoginThrottleAllowsFirstAttempt(t *testing.T) {
	throttle, _ := newTestThrottle(time.Now())
	if allowed, wait := throttle.Allow("alice"); !allowed || wait != 0 {
		t.Fatalf("Allow() = %v, %v; want true, 0", allowed, wait)
	}
}

func TestLoginThrottleBlocksAfterFailureUntilDelayElapses(t *testing.T) {
	throttle, clock := newTestThrottle(time.Now())
	throttle.RecordFailure("alice")

	if allowed, wait := throttle.Allow("alice"); allowed || wait <= 0 {
		t.Fatalf("Allow() right after a failure = %v, %v; want false, >0", allowed, wait)
	}

	*clock = clock.Add(throttleBaseDelay + time.Millisecond)
	if allowed, _ := throttle.Allow("alice"); !allowed {
		t.Fatalf("Allow() after the backoff elapsed = false, want true")
	}
}

func TestLoginThrottleDelayGrowsAndIsCapped(t *testing.T) {
	throttle, clock := newTestThrottle(time.Now())

	var lastDelay time.Duration
	for i := 0; i < 10; i++ {
		throttle.RecordFailure("alice")
		_, wait := throttle.Allow("alice")
		if i > 0 && wait < lastDelay {
			t.Fatalf("attempt %d: delay shrank (%v -> %v), want non-decreasing", i, lastDelay, wait)
		}
		if wait > throttleMaxDelay {
			t.Fatalf("attempt %d: delay %v exceeds cap %v", i, wait, throttleMaxDelay)
		}
		lastDelay = wait
		*clock = clock.Add(wait + time.Millisecond) // unblock before the next failure
	}
	if lastDelay != throttleMaxDelay {
		t.Fatalf("expected delay to reach the cap %v after repeated failures, got %v", throttleMaxDelay, lastDelay)
	}
}

func TestLoginThrottleSuccessClearsHistory(t *testing.T) {
	throttle, _ := newTestThrottle(time.Now())
	throttle.RecordFailure("alice")
	throttle.RecordSuccess("alice")

	if allowed, wait := throttle.Allow("alice"); !allowed || wait != 0 {
		t.Fatalf("Allow() after success = %v, %v; want true, 0", allowed, wait)
	}
}

func TestLoginThrottleKeysAreIndependent(t *testing.T) {
	throttle, _ := newTestThrottle(time.Now())
	throttle.RecordFailure("alice")

	if allowed, _ := throttle.Allow("bob"); !allowed {
		t.Fatalf("a failure for alice must not throttle bob")
	}
}

func TestLoginThrottleDecaysAfterQuietPeriod(t *testing.T) {
	throttle, clock := newTestThrottle(time.Now())
	throttle.RecordFailure("alice")

	*clock = clock.Add(throttleDecayAfter + time.Second)
	throttle.RecordFailure("alice") // should be treated as a fresh first failure

	_, wait := throttle.Allow("alice")
	if wait > throttleBaseDelay {
		t.Fatalf("expected the failure count to have decayed back to the base delay, got wait=%v", wait)
	}
}

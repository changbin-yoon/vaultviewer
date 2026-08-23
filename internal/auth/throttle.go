package auth

import (
	"sync"
	"time"
)

// LoginThrottle rate-limits login attempts per key (the username, in
// practice) using capped exponential backoff rather than a hard lockout.
// Each failure roughly doubles the wait before the next attempt for that
// key is allowed, up to a low ceiling, and the failure count resets after
// a period with no further failures.
//
// A hard lockout was deliberately avoided: it turns "I know a valid
// username" into a denial-of-service tool — an attacker who doesn't even
// know the password can lock the real owner out indefinitely just by
// deliberately failing a few logins. Because the wait here is capped low,
// the worst a sustained attack does to the legitimate user is force them
// to wait up to throttleMaxDelay between attempts, while a brute-force
// script attempting thousands of passwords is slowed to a crawl.
type LoginThrottle struct {
	mu    sync.Mutex
	state map[string]*throttleState
	now   func() time.Time // overridable in tests
}

type throttleState struct {
	failures  int
	lockUntil time.Time
	lastFail  time.Time
}

const (
	throttleBaseDelay  = 1 * time.Second
	throttleMaxDelay   = 30 * time.Second
	throttleDecayAfter = 15 * time.Minute
)

// NewLoginThrottle creates an empty, ready-to-use LoginThrottle.
func NewLoginThrottle() *LoginThrottle {
	return &LoginThrottle{state: make(map[string]*throttleState), now: time.Now}
}

// Allow reports whether an attempt for key may proceed right now. If not,
// it also returns how long the caller should wait before retrying.
func (t *LoginThrottle) Allow(key string) (bool, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.state[key]
	if !ok {
		return true, 0
	}
	now := t.now()
	if !now.Before(s.lockUntil) {
		return true, 0
	}
	return false, s.lockUntil.Sub(now)
}

// RecordFailure increments key's failure count and extends its backoff.
// Only call this for an actual bad-credentials result — never for an
// infrastructure error (e.g. the LDAP server being unreachable), which
// isn't the legitimate user's fault and shouldn't cost them a slot in the
// backoff schedule.
func (t *LoginThrottle) RecordFailure(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	s, ok := t.state[key]
	if !ok || now.Sub(s.lastFail) > throttleDecayAfter {
		s = &throttleState{}
		t.state[key] = s
	}
	s.failures++
	s.lastFail = now

	shift := s.failures - 1
	if shift > 5 { // cap the doubling so throttleBaseDelay<<shift can't overflow or blow past the ceiling pointlessly
		shift = 5
	}
	delay := throttleBaseDelay << uint(shift)
	if delay > throttleMaxDelay {
		delay = throttleMaxDelay
	}
	s.lockUntil = now.Add(delay)
}

// RecordSuccess clears key's failure history on a successful login.
func (t *LoginThrottle) RecordSuccess(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.state, key)
}

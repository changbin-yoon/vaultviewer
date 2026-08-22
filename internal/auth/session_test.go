package auth

import (
	"testing"
	"time"

	"github.com/vaultviewer/vaultviewer/internal/model"
)

func TestSessionManagerIssueAndVerify(t *testing.T) {
	sm := NewSessionManager([]byte("test-secret"), time.Minute)
	want := model.User{Username: "alice", Role: model.RoleDev}

	token, err := sm.Issue(want)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := sm.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if *got != want {
		t.Fatalf("got %+v, want %+v", *got, want)
	}
}

func TestSessionManagerRejectsTamperedToken(t *testing.T) {
	sm := NewSessionManager([]byte("test-secret"), time.Minute)
	token, err := sm.Issue(model.User{Username: "alice", Role: model.RoleView})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tampered := token[:len(token)-1] + "x"
	if _, err := sm.Verify(tampered); err != ErrInvalidSession {
		t.Fatalf("got err=%v, want ErrInvalidSession", err)
	}
}

func TestSessionManagerRejectsWrongSecret(t *testing.T) {
	issuer := NewSessionManager([]byte("secret-a"), time.Minute)
	verifier := NewSessionManager([]byte("secret-b"), time.Minute)

	token, err := issuer.Issue(model.User{Username: "alice", Role: model.RoleAdmin})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := verifier.Verify(token); err != ErrInvalidSession {
		t.Fatalf("got err=%v, want ErrInvalidSession", err)
	}
}

func TestSessionManagerExpiredToken(t *testing.T) {
	sm := NewSessionManager([]byte("test-secret"), -time.Minute)
	token, err := sm.Issue(model.User{Username: "alice", Role: model.RoleView})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := sm.Verify(token); err != ErrSessionExpired {
		t.Fatalf("got err=%v, want ErrSessionExpired", err)
	}
}

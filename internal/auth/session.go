package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/accesslens/accesslens/internal/model"
)

// ErrSessionExpired is returned by SessionManager.Verify for a well-formed
// but expired token.
var ErrSessionExpired = errors.New("auth: session expired")

// ErrInvalidSession is returned by SessionManager.Verify for a malformed or
// incorrectly signed token.
var ErrInvalidSession = errors.New("auth: invalid session token")

// SessionManager issues and verifies signed, stateless session tokens so
// API handlers don't need to re-run the LDAP flow on every request. Tokens
// are HMAC-SHA256 signed with secret, which must come from the environment
// (or be generated at startup) rather than ever being hardcoded.
type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

// NewSessionManager builds a SessionManager. secret must be non-empty.
func NewSessionManager(secret []byte, ttl time.Duration) *SessionManager {
	return &SessionManager{secret: secret, ttl: ttl}
}

// Issue creates a signed token encoding user's identity, role, department,
// team grants, and expiration.
func (sm *SessionManager) Issue(user model.User) (string, error) {
	expires := time.Now().Add(sm.ttl).Unix()
	teamsJSON, err := json.Marshal(user.Teams)
	if err != nil {
		return "", fmt.Errorf("encode team grants: %w", err)
	}
	payload := fmt.Sprintf("%s|%s|%s|%d|%s", user.Username, user.Role, user.Department, expires, teamsJSON)
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	sig := sm.sign(encodedPayload)
	return encodedPayload + "." + sig, nil
}

// Verify checks a token's signature and expiry and returns the User it
// encodes.
func (sm *SessionManager) Verify(token string) (*model.User, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidSession
	}
	encodedPayload, sig := parts[0], parts[1]

	expected := sm.sign(encodedPayload)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return nil, ErrInvalidSession
	}

	rawPayload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, ErrInvalidSession
	}
	fields := strings.SplitN(string(rawPayload), "|", 5)
	if len(fields) != 5 {
		return nil, ErrInvalidSession
	}
	username, role, department := fields[0], model.Role(fields[1]), fields[2]
	expires, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return nil, ErrInvalidSession
	}
	if time.Now().Unix() > expires {
		return nil, ErrSessionExpired
	}
	var teams []model.TeamGrant
	if err := json.Unmarshal([]byte(fields[4]), &teams); err != nil {
		return nil, ErrInvalidSession
	}

	return &model.User{Username: username, Role: role, Department: department, Teams: teams}, nil
}

func (sm *SessionManager) sign(encodedPayload string) string {
	mac := hmac.New(sha256.New, sm.secret)
	mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

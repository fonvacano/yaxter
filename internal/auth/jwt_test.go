package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testIssuer(t *testing.T, ttl time.Duration) *TokenIssuer {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	_, err := rand.Read(seed)
	require.NoError(t, err)
	iss, err := NewTokenIssuer("test-1", seed, ttl)
	require.NoError(t, err)
	return iss
}

func TestIssueAndVerify(t *testing.T) {
	iss := testIssuer(t, 15*time.Minute)
	token, err := iss.Issue(42)
	require.NoError(t, err)

	uid, err := iss.Verify(token)
	require.NoError(t, err)
	require.EqualValues(t, 42, uid)
}

func TestVerifyRejectsExpired(t *testing.T) {
	iss := testIssuer(t, 15*time.Minute)
	iss.now = func() time.Time { return time.Now().Add(-time.Hour) }
	token, err := iss.Issue(42)
	require.NoError(t, err)

	iss.now = time.Now
	_, err = iss.Verify(token)
	require.Error(t, err)
}

func TestVerifyRejectsForeignKeyAndTamper(t *testing.T) {
	a := testIssuer(t, 15*time.Minute)
	b := testIssuer(t, 15*time.Minute) // different key, same kid format
	token, err := a.Issue(42)
	require.NoError(t, err)

	_, err = b.Verify(token)
	require.Error(t, err, "token signed by another key must fail")

	_, err = a.Verify(token + "x")
	require.Error(t, err)
}

func TestVerifyRequiresKnownKid(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	_, err := rand.Read(seed)
	require.NoError(t, err)
	a, err := NewTokenIssuer("kid-a", seed, 15*time.Minute)
	require.NoError(t, err)
	b, err := NewTokenIssuer("kid-b", seed, 15*time.Minute) // same key, other kid
	require.NoError(t, err)

	token, err := a.Issue(7)
	require.NoError(t, err)
	_, err = b.Verify(token)
	require.Error(t, err, "unknown kid must be rejected even with the right key")
}

package auth

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenIssuer signs and verifies 15-minute EdDSA access tokens (§2.8).
// Verification is keyed by the `kid` header so keys can rotate: new tokens
// sign with the current key while old kids stay verifiable until expiry.
// The seed source is pluggable by the caller (env in demo, Lockbox in prod).
type TokenIssuer struct {
	kid  string
	priv ed25519.PrivateKey
	pubs map[string]ed25519.PublicKey
	ttl  time.Duration
	now  func() time.Time
}

func NewTokenIssuer(kid string, seed []byte, ttl time.Duration) (*TokenIssuer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("auth: jwt seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return &TokenIssuer{
		kid:  kid,
		priv: priv,
		pubs: map[string]ed25519.PublicKey{kid: priv.Public().(ed25519.PublicKey)},
		ttl:  ttl,
		now:  time.Now,
	}, nil
}

// TTLSeconds is the access-token lifetime for the expires_in response field.
func (i *TokenIssuer) TTLSeconds() int { return int(i.ttl.Seconds()) }

func (i *TokenIssuer) Issue(userID int64) (string, error) {
	now := i.now()
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
	})
	tok.Header["kid"] = i.kid
	return tok.SignedString(i.priv)
}

func (i *TokenIssuer) Verify(token string) (int64, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{},
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			pub, ok := i.pubs[kid]
			if !ok {
				return nil, fmt.Errorf("auth: unknown kid %q", kid)
			}
			return pub, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithTimeFunc(func() time.Time { return i.now() }),
	)
	if err != nil {
		return 0, err
	}
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return 0, errors.New("auth: missing subject")
	}
	return strconv.ParseInt(claims.Subject, 10, 64)
}

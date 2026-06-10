package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// registerAndToken registers a fresh user and returns a bearer access token.
func registerAndToken(t *testing.T, h http.Handler, username string) string {
	t.Helper()
	reg := postJSON(t, h, "/v1/auth/register", map[string]any{
		"username": username,
		"email":    username + "@example.com",
		"password": "password123",
	}, map[string]string{"Idempotency-Key": uuid.NewString()})
	require.Equal(t, http.StatusCreated, reg.Code, reg.Body.String())
	var body struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(reg.Body.Bytes(), &body))
	require.NotEmpty(t, body.Tokens.AccessToken)
	return body.Tokens.AccessToken
}

// TestDuplicateIdempotencyKeySingleTweet verifies the Idempotency-Key replay
// contract for POST /v1/tweets: the second request with the same key returns
// the byte-identical cached response, and only one row and one event exist.
func TestDuplicateIdempotencyKeySingleTweet(t *testing.T) {
	h, pool := liveHandlerAndPool(t, 100)
	ctx := t.Context()

	tok := registerAndToken(t, h, "writer")
	key := uuid.NewString()

	body := map[string]any{"text": "exactly once"}
	hdrs := map[string]string{
		"Idempotency-Key": key,
		"Authorization":   "Bearer " + tok,
	}

	first := postJSON(t, h, "/v1/tweets", body, hdrs)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	second := postJSON(t, h, "/v1/tweets", body, hdrs)
	require.Equal(t, http.StatusCreated, second.Code)
	require.Equal(t, first.Body.String(), second.Body.String(),
		"replay must return the byte-identical response")

	// Single row, single tweets.v1 event.
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM tweets`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='tweets.v1'`).Scan(&n))
	require.Equal(t, 1, n)
}

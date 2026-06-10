package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYandexAuthURLCarriesStateAndPKCE(t *testing.T) {
	p := NewYandex("client-id", "client-secret", YandexEndpoints{})
	raw := p.AuthCodeURL("state-1", "challenge-1", "https://app/cb")
	u, err := url.Parse(raw)
	require.NoError(t, err)
	q := u.Query()
	require.Equal(t, "state-1", q.Get("state"))
	require.Equal(t, "challenge-1", q.Get("code_challenge"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.Equal(t, "client-id", q.Get("client_id"))
	require.Equal(t, "https://app/cb", q.Get("redirect_uri"))
	require.Equal(t, "code", q.Get("response_type"))
}

func TestYandexExchangeAndIdentity(t *testing.T) {
	var gotVerifier string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			gotVerifier = r.Form.Get("code_verifier")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at-1", "token_type": "bearer", "expires_in": 3600,
			})
		case "/info":
			require.Equal(t, "OAuth at-1", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "y-123", "login": "vasya", "default_email": "vasya@yandex.ru",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mock.Close)

	p := NewYandex("client-id", "client-secret", YandexEndpoints{
		TokenURL: mock.URL + "/token", InfoURL: mock.URL + "/info",
	})
	tok, err := p.Exchange(context.Background(), "code-1", "verifier-1", "https://app/cb")
	require.NoError(t, err)
	require.Equal(t, "verifier-1", gotVerifier, "PKCE verifier must be sent")

	ident, err := p.Identity(context.Background(), tok)
	require.NoError(t, err)
	require.Equal(t, "y-123", ident.ProviderUserID)
	require.Equal(t, "vasya", ident.Login)
	require.Equal(t, "vasya@yandex.ru", ident.Email)
	require.True(t, ident.EmailVerified,
		"yandex default_email is verified by construction (§2.8)")
}

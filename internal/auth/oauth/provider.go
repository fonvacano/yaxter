// Package oauth implements social login (ARCHITECTURE.md §2.8): providers
// authenticate, we issue our own tokens. Provider tokens are used once to
// fetch the profile and discarded — never to authorize API calls.
package oauth

import "context"

// Identity is what a provider asserts about the user.
type Identity struct {
	ProviderUserID string
	Email          string
	EmailVerified  bool // gates silent auto-link (§2.8 linking rule 2)
	Login          string
}

// Token carries the provider exchange result; IDToken is set by OIDC
// providers (Google) and empty for plain OAuth 2.0 (Yandex).
type Token struct {
	AccessToken string
	IDToken     string
}

// Provider is the §2.8 OAuthProvider abstraction; adding VK/Apple later is
// a config entry plus one implementation of this interface.
type Provider interface {
	Name() string
	DisplayName() string
	AuthCodeURL(state, codeChallenge, redirectURI string) string
	Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*Token, error)
	Identity(ctx context.Context, tok *Token) (Identity, error)
}

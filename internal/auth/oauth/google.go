package oauth

import (
	"context"
	"errors"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Google is full OIDC: the id_token is verified against the issuer's JWKS
// (§2.8). The issuer is configurable so tests run against a mock; production
// uses https://accounts.google.com.
type Google struct {
	cfg      oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func NewGoogle(ctx context.Context, clientID, clientSecret, issuer string) (*Google, error) {
	if issuer == "" {
		issuer = "https://accounts.google.com"
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	return &Google{
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

func (g *Google) Name() string        { return "google" }
func (g *Google) DisplayName() string { return "Google" }

func (g *Google) AuthCodeURL(state, codeChallenge, redirectURI string) string {
	cfg := g.cfg
	cfg.RedirectURL = redirectURI
	return cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"))
}

func (g *Google) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*Token, error) {
	cfg := g.cfg
	cfg.RedirectURL = redirectURI
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return nil, err
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return nil, errors.New("oauth: google response missing id_token")
	}
	return &Token{AccessToken: tok.AccessToken, IDToken: raw}, nil
}

func (g *Google) Identity(ctx context.Context, tok *Token) (Identity, error) {
	idToken, err := g.verifier.Verify(ctx, tok.IDToken) // signature, aud, exp
	if err != nil {
		return Identity{}, err
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, err
	}
	return Identity{
		ProviderUserID: idToken.Subject,
		Email:          claims.Email,
		EmailVerified:  claims.EmailVerified, // google asserts it (§2.8 rule 2)
		Login:          claims.Name,
	}, nil
}

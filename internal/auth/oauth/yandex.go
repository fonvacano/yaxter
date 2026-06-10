package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

// YandexEndpoints are overridable for tests/dev; zero values use production.
type YandexEndpoints struct {
	AuthURL  string
	TokenURL string
	InfoURL  string
}

type Yandex struct {
	cfg     oauth2.Config
	infoURL string
}

func NewYandex(clientID, clientSecret string, ep YandexEndpoints) *Yandex {
	if ep.AuthURL == "" {
		ep.AuthURL = "https://oauth.yandex.ru/authorize"
	}
	if ep.TokenURL == "" {
		ep.TokenURL = "https://oauth.yandex.ru/token"
	}
	if ep.InfoURL == "" {
		ep.InfoURL = "https://login.yandex.ru/info"
	}
	return &Yandex{
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     oauth2.Endpoint{AuthURL: ep.AuthURL, TokenURL: ep.TokenURL},
		},
		infoURL: ep.InfoURL,
	}
}

func (y *Yandex) Name() string        { return "yandex" }
func (y *Yandex) DisplayName() string { return "Yandex" }

func (y *Yandex) AuthCodeURL(state, codeChallenge, redirectURI string) string {
	cfg := y.cfg
	cfg.RedirectURL = redirectURI
	return cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"))
}

func (y *Yandex) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*Token, error) {
	cfg := y.cfg
	cfg.RedirectURL = redirectURI
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return nil, err
	}
	return &Token{AccessToken: tok.AccessToken}, nil
}

func (y *Yandex) Identity(ctx context.Context, tok *Token) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		y.infoURL+"?format=json", nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Authorization", "OAuth "+tok.AccessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer res.Body.Close() //nolint:errcheck
	if res.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("oauth: yandex info returned %d", res.StatusCode)
	}
	var info struct {
		ID           string `json:"id"`
		Login        string `json:"login"`
		DefaultEmail string `json:"default_email"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return Identity{}, err
	}
	return Identity{
		ProviderUserID: info.ID,
		Login:          info.Login,
		Email:          info.DefaultEmail,
		// default_email is Yandex-verified by construction (§2.8 rule 2).
		EmailVerified: info.DefaultEmail != "",
	}, nil
}

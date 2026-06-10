package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func mockOIDCIssuer(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "ghcr.io/navikt/mock-oauth2-server:2.1.10",
			ExposedPorts: []string{"8080/tcp"},
			Env: map[string]string{
				"JSON_CONFIG": `{"interactiveLogin": false}`,
			},
			WaitingFor: wait.ForHTTP("/default/.well-known/openid-configuration").WithPort("8080/tcp"),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	host, err := ctr.Host(ctx)
	require.NoError(t, err)
	port, err := ctr.MappedPort(ctx, "8080")
	require.NoError(t, err)
	return fmt.Sprintf("http://%s:%s/default", host, port.Port())
}

func TestGoogleFullCodeDanceAgainstMock(t *testing.T) {
	issuer := mockOIDCIssuer(t)
	ctx := context.Background()

	p, err := NewGoogle(ctx, "test-client", "test-secret", issuer)
	require.NoError(t, err)
	require.Equal(t, "google", p.Name())

	// Non-interactive mock: GET /authorize 302s straight back with a code.
	authURL := p.AuthCodeURL("state-1", "challenge-ignored-by-mock", "http://127.0.0.1/cb")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(authURL)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusFound, res.StatusCode)
	loc, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)
	code := loc.Query().Get("code")
	require.NotEmpty(t, code)
	require.Equal(t, "state-1", loc.Query().Get("state"))

	tok, err := p.Exchange(ctx, code, "challenge-ignored-by-mock", "http://127.0.0.1/cb")
	require.NoError(t, err)
	require.NotEmpty(t, tok.IDToken, "google adapter must capture the id_token")

	ident, err := p.Identity(ctx, tok)
	require.NoError(t, err)
	require.NotEmpty(t, ident.ProviderUserID, "sub claim becomes the provider user id")
	require.True(t, strings.HasPrefix(issuer, "http://"), "sanity: talked to the mock")
}

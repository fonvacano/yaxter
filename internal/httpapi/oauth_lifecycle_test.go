package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// liveHandler wires no OAuth providers by default, so every provider is
// "disabled" — the DoD's disabled-provider 404 case.
func TestDisabledProviderIs404(t *testing.T) {
	h := liveHandler(t, 100)
	for _, path := range []string{
		"/v1/auth/oauth/google/start",
		"/v1/auth/oauth/google/callback?code=x&state=y",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNotFound, rr.Code, path)
	}
}

func TestProvidersDiscoveryEmptyWithoutConfig(t *testing.T) {
	h := liveHandler(t, 100)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/providers", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.JSONEq(t, `{"providers":[]}`, rr.Body.String())
}

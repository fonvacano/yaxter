package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterRejectsMalformedBody(t *testing.T) {
	h := &AuthHandlers{svc: nil} // svc must not be reached
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register",
		strings.NewReader(`{not json`))
	rr := httptest.NewRecorder()
	h.Register(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRegisterRejectsInvalidFields(t *testing.T) {
	h := &AuthHandlers{svc: nil}
	for _, body := range []string{
		`{"username":"x","email":"a@b.c","password":"longenough1"}`,         // username too short
		`{"username":"valid_name","email":"nope","password":"longenough1"}`, // bad email
		`{"username":"valid_name","email":"a@b.c","password":"short"}`,      // short password
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/register",
			strings.NewReader(body))
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		require.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", body)
	}
}

func TestRefreshCookieRoundtrip(t *testing.T) {
	rr := httptest.NewRecorder()
	setRefreshCookie(rr, "tok-123")
	res := rr.Result()
	cookies := res.Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	require.Equal(t, refreshCookieName, c.Name)
	require.True(t, c.HttpOnly)
	require.Equal(t, http.SameSiteStrictMode, c.SameSite)
	require.Equal(t, "/v1/auth", c.Path)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", nil)
	req.AddCookie(c)
	require.Equal(t, "tok-123", refreshTokenFrom(req, ""))
	require.Equal(t, "body-wins", refreshTokenFrom(req, "body-wins"))
}

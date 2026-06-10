package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/fonvacano/yaxter/internal/auth"
)

const refreshCookieName = "yx_refresh"

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,30}$`)

type AuthHandlers struct {
	svc *auth.Service
}

// setRefreshCookie delivers the refresh token to web clients
// (HttpOnly; SameSite=Strict; scoped to the auth routes, §7). The token is
// also returned in the body for non-web clients — the contract allows both.
func setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/v1/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
}

// refreshTokenFrom prefers an explicit body token, falling back to the cookie.
func refreshTokenFrom(r *http.Request, bodyToken string) string {
	if bodyToken != "" {
		return bodyToken
	}
	if c, err := r.Cookie(refreshCookieName); err == nil {
		return c.Value
	}
	return ""
}

func (h *AuthHandlers) tokenPairBody(p auth.TokenPair) TokenPair {
	tt := TokenPairTokenType(Bearer)
	refresh := p.Refresh
	return TokenPair{
		AccessToken:  p.Access,
		TokenType:    tt,
		ExpiresIn:    p.ExpiresIn,
		RefreshToken: &refresh,
	}
}

func privateUser(u auth.User, hasPassword bool) PrivateUser {
	return PrivateUser{
		Id:              formatID(u.ID),
		Username:        u.Username,
		Bio:             u.Bio,
		FollowersCount:  0,
		FollowingCount:  0,
		CreatedAt:       u.CreatedAt,
		Email:           openapi_types.Email(u.Email),
		HasPassword:     hasPassword,
		LinkedProviders: []string{},
		AvatarUrl:       nil,
	}
}

func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "invalid JSON")
		return
	}
	if !usernameRe.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid username")
		return
	}
	if _, err := mail.ParseAddress(string(req.Email)); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid email")
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 128 {
		writeError(w, http.StatusBadRequest, "validation_failed", "password must be 8-128 chars")
		return
	}
	u, pair, err := h.svc.Register(r.Context(), req.Username, string(req.Email), req.Password)
	switch {
	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "already_exists", "username or email already taken")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "registration failed")
		return
	}
	setRefreshCookie(w, pair.Refresh)
	writeJSON(w, http.StatusCreated, AuthResponse{
		User: privateUser(u, true), Tokens: h.tokenPairBody(pair),
	})
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "invalid JSON")
		return
	}
	u, pair, err := h.svc.Login(r.Context(), req.Login, req.Password)
	if err != nil { // uniform 401 for every failure mode (§7)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
		return
	}
	setRefreshCookie(w, pair.Refresh)
	writeJSON(w, http.StatusOK, AuthResponse{
		User: privateUser(u, true), Tokens: h.tokenPairBody(pair),
	})
}

func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // body optional
	bodyToken := ""
	if req.RefreshToken != nil {
		bodyToken = *req.RefreshToken
	}
	token := refreshTokenFrom(r, bodyToken)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "invalid_refresh", "missing refresh token")
		return
	}
	pair, err := h.svc.Refresh(r.Context(), token)
	if err != nil { // ErrReused and ErrInvalidRefresh are both uniform 401s
		writeError(w, http.StatusUnauthorized, "invalid_refresh", "invalid refresh token")
		return
	}
	setRefreshCookie(w, pair.Refresh)
	writeJSON(w, http.StatusOK, h.tokenPairBody(pair))
}

func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	token := refreshTokenFrom(r, "")
	if token != "" {
		_ = h.svc.Logout(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookieName, Value: "", Path: "/v1/auth", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

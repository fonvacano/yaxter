package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fonvacano/yaxter/internal/auth"
	"github.com/fonvacano/yaxter/internal/auth/oauth"
)

type OAuthHandlers struct {
	flow *oauth.Flow
	svc  *auth.Service
}

func (h *OAuthHandlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	out := ProviderList{Providers: []OAuthProviderInfo{}}
	for name, p := range h.flow.Providers() {
		out.Providers = append(out.Providers, OAuthProviderInfo{
			Name: name, DisplayName: p.DisplayName(),
			StartUrl: "/v1/auth/oauth/" + name + "/start",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// safeRedirect allows only same-app relative paths (open-redirect guard, §7).
func safeRedirect(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	return raw
}

func (h *OAuthHandlers) Start(w http.ResponseWriter, r *http.Request, provider string) {
	redirectTo := safeRedirect(r.URL.Query().Get("redirect_to"))
	authURL, err := h.flow.Start(r.Context(), provider, redirectTo, oauth.ModeLogin, 0)
	if errors.Is(err, oauth.ErrUnknownProvider) {
		writeError(w, http.StatusNotFound, "not_found", "provider not enabled")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "oauth start failed")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *OAuthHandlers) Callback(w http.ResponseWriter, r *http.Request, provider string) {
	q := r.URL.Query()
	userID, err := h.flow.Callback(r.Context(), provider, q.Get("code"), q.Get("state"))
	switch {
	case errors.Is(err, oauth.ErrUnknownProvider):
		writeError(w, http.StatusNotFound, "not_found", "provider not enabled")
		return
	case errors.Is(err, oauth.ErrInvalidState):
		writeError(w, http.StatusBadRequest, "invalid_state", "state is invalid, expired, or already used")
		return
	case errors.Is(err, oauth.ErrLinkRequired):
		// Careful wording: must not confirm the email exists (§7).
		writeError(w, http.StatusBadRequest, "link_required",
			"cannot complete social sign-in; if you have an account, log in and link this provider in settings")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "oauth_failed", "social sign-in failed")
		return
	}
	pair, err := h.svc.TokenPairFor(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token issuance failed")
		return
	}
	info, providers, err := h.svc.UserInfo(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "profile load failed")
		return
	}
	setRefreshCookie(w, pair.Refresh)
	refresh := pair.Refresh
	writeJSON(w, http.StatusOK, AuthResponse{
		User: privateUserFromInfo(info, providers),
		Tokens: TokenPair{
			AccessToken: pair.Access, TokenType: TokenPairTokenType(Bearer),
			ExpiresIn: pair.ExpiresIn, RefreshToken: &refresh,
		},
	})
}

func (h *OAuthHandlers) Link(w http.ResponseWriter, r *http.Request, provider string) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	authURL, err := h.flow.Start(r.Context(), provider, "/settings", oauth.ModeLink, uid)
	if errors.Is(err, oauth.ErrUnknownProvider) {
		writeError(w, http.StatusNotFound, "not_found", "provider not enabled")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "link start failed")
		return
	}
	writeJSON(w, http.StatusOK, LinkStartResponse{AuthUrl: authURL})
}

func (h *OAuthHandlers) Unlink(w http.ResponseWriter, r *http.Request, provider string) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	switch err := h.flow.Unlink(r.Context(), uid, provider); {
	case errors.Is(err, oauth.ErrLastCredential):
		writeError(w, http.StatusConflict, "last_credential",
			"set a password before unlinking your only sign-in method")
	case err != nil:
		writeError(w, http.StatusNotFound, "not_found", "provider not linked")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

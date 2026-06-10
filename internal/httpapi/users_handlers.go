package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/fonvacano/yaxter/internal/users"
)

type UsersHandlers struct {
	svc          *users.Service
	mediaBaseURL string
}

func (h *UsersHandlers) avatarURL(key *string) *string {
	if key == nil {
		return nil
	}
	u := h.mediaBaseURL + "/feed/" + *key + ".webp"
	return &u
}

func (h *UsersHandlers) toUser(u users.User) User {
	return User{
		Id:             formatID(u.ID),
		Username:       u.Username,
		Bio:            u.Bio,
		AvatarUrl:      h.avatarURL(u.AvatarKey),
		FollowersCount: u.FollowersCount,
		FollowingCount: u.FollowingCount,
		CreatedAt:      u.CreatedAt,
	}
}

func (h *UsersHandlers) toPrivateUser(u users.User) PrivateUser {
	return PrivateUser{
		Id:              formatID(u.ID),
		Username:        u.Username,
		Bio:             u.Bio,
		Email:           openapi_types.Email(u.Email),
		AvatarUrl:       h.avatarURL(u.AvatarKey),
		FollowersCount:  u.FollowersCount,
		FollowingCount:  u.FollowingCount,
		HasPassword:     u.HasPassword,
		LinkedProviders: []string{},
		CreatedAt:       u.CreatedAt,
	}
}

func (h *UsersHandlers) toUserSummary(s users.Summary) UserSummary {
	return UserSummary{
		Id:        formatID(s.ID),
		Username:  s.Username,
		AvatarUrl: h.avatarURL(s.AvatarKey),
	}
}

func (h *UsersHandlers) GetMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	u, err := h.svc.GetByID(r.Context(), uid)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.toPrivateUser(u))
}

func (h *UsersHandlers) UpdateMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	up := users.UpdateProfile{Bio: req.Bio}
	if req.AvatarMediaId != nil {
		id, err := strconv.ParseInt(*req.AvatarMediaId, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_media_id", "avatar_media_id must be a decimal int64")
			return
		}
		up.AvatarMediaID = &id
	}
	u, err := h.svc.UpdateProfile(r.Context(), uid, up)
	if err != nil {
		if errors.Is(err, users.ErrMediaNotReady) {
			writeError(w, http.StatusUnprocessableEntity, "media_not_ready", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.toPrivateUser(u))
}

func (h *UsersHandlers) GetUser(w http.ResponseWriter, r *http.Request, username Username) {
	u, err := h.svc.GetByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.toUser(u))
}

func (h *UsersHandlers) FollowUser(w http.ResponseWriter, r *http.Request, username Username) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if err := h.svc.Follow(r.Context(), uid, username); err != nil {
		switch {
		case errors.Is(err, users.ErrSelfFollow):
			writeError(w, http.StatusUnprocessableEntity, "self_follow", err.Error())
		case errors.Is(err, users.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "user not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandlers) UnfollowUser(w http.ResponseWriter, r *http.Request, username Username) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if err := h.svc.Unfollow(r.Context(), uid, username); err != nil {
		switch {
		case errors.Is(err, users.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "user not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandlers) ListFollowers(w http.ResponseWriter, r *http.Request, username Username, params ListFollowersParams) {
	h.listEdge(w, r, username, params.Cursor, params.Limit, false)
}

func (h *UsersHandlers) ListFollowing(w http.ResponseWriter, r *http.Request, username Username, params ListFollowingParams) {
	h.listEdge(w, r, username, params.Cursor, params.Limit, true)
}

func (h *UsersHandlers) listEdge(w http.ResponseWriter, r *http.Request, username string, cursorP *Cursor, limitP *Limit, following bool) {
	limit := 20
	if limitP != nil && *limitP > 0 && *limitP <= 100 {
		limit = *limitP
	}
	var cursor int64
	if cursorP != nil && *cursorP != "" {
		id, err := strconv.ParseInt(*cursorP, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor must be a decimal int64")
			return
		}
		cursor = id
	}

	var (
		items []users.Summary
		next  int64
		err   error
	)
	if following {
		items, next, err = h.svc.Following(r.Context(), username, cursor, limit)
	} else {
		items, next, err = h.svc.Followers(r.Context(), username, cursor, limit)
	}
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	summaries := make([]UserSummary, len(items))
	for i, s := range items {
		summaries[i] = h.toUserSummary(s)
	}
	resp := UserPage{Items: summaries}
	if next != 0 {
		c := strconv.FormatInt(next, 10)
		resp.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, resp)
}

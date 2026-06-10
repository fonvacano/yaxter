package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/tweets"
)

// TweetsHandlers implements the tweet write/read endpoints.
type TweetsHandlers struct {
	svc          *tweets.Service
	mediaBaseURL string
}

func (h *TweetsHandlers) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req CreateTweetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "invalid JSON")
		return
	}

	var retweetOfID int64
	if req.RetweetOfId != nil {
		var err error
		retweetOfID, err = strconv.ParseInt(*req.RetweetOfId, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_failed", "invalid retweet_of_id")
			return
		}
	}

	var mediaIDs []int64
	if req.MediaIds != nil {
		for _, mid := range *req.MediaIds {
			id, err := strconv.ParseInt(mid, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "validation_failed", "invalid media_id")
				return
			}
			mediaIDs = append(mediaIDs, id)
		}
	}

	tw, err := h.svc.Create(r.Context(), uid, req.Text, mediaIDs, retweetOfID)
	if err != nil {
		switch {
		case errors.Is(err, tweets.ErrValidation):
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		case errors.Is(err, tweets.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "retweet target not found")
		case errors.Is(err, tweets.ErrMediaNotReady):
			writeError(w, http.StatusUnprocessableEntity, "media_not_ready", "one or more media items are not ready")
		default:
			writeError(w, http.StatusInternalServerError, "internal", "could not create tweet")
		}
		return
	}

	got, err := h.svc.Get(r.Context(), tw.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not hydrate tweet")
		return
	}
	writeJSON(w, http.StatusCreated, h.hydrate(got))
}

func (h *TweetsHandlers) Get(w http.ResponseWriter, r *http.Request, id string) {
	tweetID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid tweet id")
		return
	}

	got, err := h.svc.Get(r.Context(), tweetID)
	if err != nil {
		if errors.Is(err, tweets.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "tweet not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not fetch tweet")
		return
	}
	writeJSON(w, http.StatusOK, h.hydrate(got))
}

func (h *TweetsHandlers) Delete(w http.ResponseWriter, r *http.Request, id string) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	tweetID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid tweet id")
		return
	}

	err = h.svc.Delete(r.Context(), uid, tweetID)
	if err != nil {
		switch {
		case errors.Is(err, tweets.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "tweet not found")
		case errors.Is(err, tweets.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "not the author")
		default:
			writeError(w, http.StatusInternalServerError, "internal", "could not delete tweet")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hydrate converts a HydratedTweet to the generated API Tweet type.
func (h *TweetsHandlers) hydrate(ht tweets.HydratedTweet) Tweet {
	var avatarURL *string
	if ht.AuthorAvatarKey != nil && h.mediaBaseURL != "" {
		u := h.mediaBaseURL + "/orig/" + *ht.AuthorAvatarKey
		avatarURL = &u
	}

	var mediaRefs *[]MediaRef
	if len(ht.MediaIDs) > 0 {
		refs := make([]MediaRef, len(ht.MediaIDs))
		for i, mid := range ht.MediaIDs {
			sid := formatID(mid)
			base := h.mediaBaseURL + "/orig/" + sid
			refs[i] = MediaRef{
				Id: sid,
				Urls: struct {
					Feed  string `json:"feed"`
					Orig  string `json:"orig"`
					Thumb string `json:"thumb"`
				}{
					Feed:  h.mediaBaseURL + "/feed/" + sid,
					Orig:  base,
					Thumb: h.mediaBaseURL + "/thumb/" + sid,
				},
			}
		}
		mediaRefs = &refs
	}

	return Tweet{
		Id:            formatID(ht.ID),
		Text:          ht.Text,
		CreatedAt:     ht.CreatedAt,
		LikesCount:    ht.LikesCount,
		RetweetsCount: ht.RetweetsCount,
		Author: UserSummary{
			Id:        formatID(ht.AuthorID),
			Username:  ht.AuthorUsername,
			AvatarUrl: avatarURL,
		},
		Media: mediaRefs,
	}
}

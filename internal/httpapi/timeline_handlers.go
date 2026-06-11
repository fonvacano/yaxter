package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/timeline"
	"github.com/fonvacano/yaxter/internal/tweets"
)

type TimelineHandlers struct {
	svc          *timeline.Service
	mediaBaseURL string
}

func (h *TimelineHandlers) Home(w http.ResponseWriter, r *http.Request, params GetHomeTimelineParams) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	cursor, limit := parsePage(params.Cursor, params.Limit)
	items, next, err := h.svc.Home(r.Context(), uid, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load timeline")
		return
	}
	h.writePage(w, items, next)
}

func (h *TimelineHandlers) UserTweets(w http.ResponseWriter, r *http.Request, username string, params GetUserTweetsParams) {
	cursor, limit := parsePage(params.Cursor, params.Limit)
	items, next, err := h.svc.Profile(r.Context(), username, cursor, limit)
	if err != nil {
		if errors.Is(err, timeline.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load profile timeline")
		return
	}
	h.writePage(w, items, next)
}

func (h *TimelineHandlers) writePage(w http.ResponseWriter, items []tweets.HydratedTweet, next *int64) {
	out := make([]Tweet, 0, len(items))
	for _, ht := range items {
		out = append(out, tweetToAPI(ht, h.mediaBaseURL))
	}
	resp := TweetPage{Items: out}
	if next != nil {
		c := strconv.FormatInt(*next, 10)
		resp.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, resp)
}

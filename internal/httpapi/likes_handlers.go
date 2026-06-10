package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/tweets"
)

func (h *TweetsHandlers) setLike(w http.ResponseWriter, r *http.Request, id string, like bool) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	tweetID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
		return
	}
	if like {
		err = h.svc.Like(r.Context(), uid, tweetID)
	} else {
		err = h.svc.Unlike(r.Context(), uid, tweetID)
	}
	switch {
	case errors.Is(err, tweets.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such tweet")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "like failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

package httpapi

import (
	"net/http"

	"github.com/fonvacano/yaxter/internal/tweets"
)

// TweetsHandlers holds the tweets service and handles tweet HTTP requests.
type TweetsHandlers struct {
	svc *tweets.Service
}

// requireUser extracts the authenticated user id; writes 401 and returns false if absent.
func requireUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return 0, false
	}
	return uid, true
}

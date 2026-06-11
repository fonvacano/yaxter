package httpapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHomeTimelineRequiresAuth(t *testing.T) {
	h := liveHandler(t, 100)
	rr := getJSON(t, h, "/v1/timeline", nil)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHomeTimelineEmptyForNewUser(t *testing.T) {
	h := liveHandler(t, 100)
	token := registerAndLogin(t, h, "tluser")
	rr := getJSON(t, h, "/v1/timeline",
		map[string]string{"Authorization": "Bearer " + token})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `"items":[]`)
}

func TestUserTweetsUnknownUser404(t *testing.T) {
	h := liveHandler(t, 100)
	rr := getJSON(t, h, "/v1/users/ghost/tweets", nil)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

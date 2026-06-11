package httpapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationsEndpointRequiresAuth(t *testing.T) {
	h := liveHandler(t, 100) // integration helper; skips on -short
	rr := getJSON(t, h, "/v1/notifications", nil)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestUnreadCountEndpointForAuthedUser(t *testing.T) {
	h := liveHandler(t, 100)
	token := registerAndLogin(t, h, "noteuser")
	rr := getJSON(t, h, "/v1/notifications/unread_count",
		map[string]string{"Authorization": "Bearer " + token})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `"count":0`)
}

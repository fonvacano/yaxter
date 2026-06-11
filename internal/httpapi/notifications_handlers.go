package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/notifications"
)

type NotificationsHandlers struct {
	svc          *notifications.Service
	mediaBaseURL string
}

func (h *NotificationsHandlers) List(w http.ResponseWriter, r *http.Request, params ListNotificationsParams) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	cursor, limit := parsePage(params.Cursor, params.Limit)

	items, next, err := h.svc.List(r.Context(), uid, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list notifications")
		return
	}

	out := make([]Notification, len(items))
	for i, n := range items {
		out[i] = h.toAPI(n)
	}
	resp := NotificationPage{Items: out}
	if next != nil {
		c := strconv.FormatInt(*next, 10)
		resp.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *NotificationsHandlers) UnreadCount(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	n, err := h.svc.UnreadCount(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not count notifications")
		return
	}
	writeJSON(w, http.StatusOK, UnreadCount{Count: n})
}

func (h *NotificationsHandlers) MarkRead(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req MarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "invalid JSON")
		return
	}
	upTo, err := strconv.ParseInt(string(req.UpToId), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid up_to_id")
		return
	}
	if err := h.svc.MarkRead(r.Context(), uid, upTo); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not mark read")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificationsHandlers) toAPI(n notifications.Notification) Notification {
	var avatarURL *string
	if n.ActorAvatar != nil && h.mediaBaseURL != "" {
		u := h.mediaBaseURL + "/orig/" + *n.ActorAvatar
		avatarURL = &u
	}
	var subject *Id
	if n.SubjectID != nil {
		s := Id(formatID(*n.SubjectID))
		subject = &s
	}
	return Notification{
		Id:        Id(formatID(n.ID)),
		Kind:      NotificationKind(n.Kind),
		Read:      n.ReadAt != nil,
		CreatedAt: n.CreatedAt,
		SubjectId: subject,
		Actor: UserSummary{
			Id:        formatID(n.ActorID),
			Username:  n.ActorName,
			AvatarUrl: avatarURL,
		},
	}
}

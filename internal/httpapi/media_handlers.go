package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/fonvacano/yaxter/internal/media"
)

type MediaHandlers struct {
	svc          *media.Service
	mediaBaseURL string
}

func (h *MediaHandlers) toAPI(m media.Media) Media {
	out := Media{Id: formatID(m.ID), Status: MediaStatus(m.Status)}
	if m.Status == "ready" {
		feed := media.URL(h.mediaBaseURL, "feed", m.ID)
		orig := media.URL(h.mediaBaseURL, "orig", m.ID)
		thumb := media.URL(h.mediaBaseURL, "thumb", m.ID)
		out.Urls = &struct {
			Feed  *string `json:"feed,omitempty"`
			Orig  *string `json:"orig,omitempty"`
			Thumb *string `json:"thumb,omitempty"`
		}{Feed: &feed, Orig: &orig, Thumb: &thumb}
	}
	return out
}

func (h *MediaHandlers) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req CreateMediaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "invalid JSON")
		return
	}
	ticket, err := h.svc.Create(r.Context(), uid, string(req.ContentType), int64(req.SizeBytes))
	switch {
	case errors.Is(err, media.ErrBadType), errors.Is(err, media.ErrTooLarge):
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "media allocation failed")
		return
	}
	writeJSON(w, http.StatusCreated, MediaUploadTicket{
		MediaId:   formatID(ticket.MediaID),
		UploadUrl: ticket.UploadURL,
		ExpiresAt: ticket.ExpiresAt,
	})
}

func (h *MediaHandlers) Get(w http.ResponseWriter, r *http.Request, id string) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	mediaID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such media")
		return
	}
	m, err := h.svc.Get(r.Context(), uid, mediaID)
	if errors.Is(err, media.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such media")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "media read failed")
		return
	}
	writeJSON(w, http.StatusOK, h.toAPI(m))
}

func (h *MediaHandlers) Complete(w http.ResponseWriter, r *http.Request, id string) {
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	mediaID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such media")
		return
	}
	m, err := h.svc.Complete(r.Context(), uid, mediaID)
	switch {
	case errors.Is(err, media.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such media")
	case errors.Is(err, media.ErrNoObject):
		writeError(w, http.StatusConflict, "not_uploaded", "object not found in storage")
	case errors.Is(err, media.ErrSizeMismatch):
		writeError(w, http.StatusConflict, "size_mismatch", "uploaded size differs from declared")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "complete failed")
	default:
		writeJSON(w, http.StatusOK, h.toAPI(m))
	}
}

package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/shridarpatil/bifrost/config"
	"github.com/shridarpatil/bifrost/store"
)

type MediaHandler struct {
	mediaStore *store.MediaStore
	cfg        *config.Config
}

func NewMediaHandler(mediaStore *store.MediaStore, cfg *config.Config) *MediaHandler {
	return &MediaHandler{
		mediaStore: mediaStore,
		cfg:        cfg,
	}
}

func (h *MediaHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v{version}/{phoneID}/media", h.Upload)
	r.Get("/v{version}/{mediaID}", h.GetMediaInfo)
	r.Get("/media/{mediaID}", h.Download)
}

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(int64(h.cfg.Media.MaxSizeMB) << 20); err != nil {
		h.sendError(w, "Failed to parse multipart form", 400)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.sendError(w, "No file provided", 400)
		return
	}
	defer file.Close()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = r.FormValue("type")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	mediaType := r.FormValue("messaging_product")
	if mediaType == "" {
		mediaType = "whatsapp"
	}

	mediaID, err := h.mediaStore.StoreFromReader(file, mimeType, getMediaType(mimeType))
	if err != nil {
		h.sendError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id": mediaID,
	})
}

func (h *MediaHandler) GetMediaInfo(w http.ResponseWriter, r *http.Request) {
	mediaID := chi.URLParam(r, "mediaID")

	metadata, err := h.mediaStore.Get(mediaID)
	if err != nil {
		h.sendError(w, "Media not found", 404)
		return
	}

	baseURL := getBaseURL(r)
	downloadURL := fmt.Sprintf("%s/media/%s", baseURL, mediaID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messaging_product": "whatsapp",
		"url":               downloadURL,
		"mime_type":         metadata.MimeType,
		"sha256":            metadata.SHA256,
		"file_size":         metadata.Size,
		"id":                mediaID,
	})
}

func (h *MediaHandler) Download(w http.ResponseWriter, r *http.Request) {
	mediaID := chi.URLParam(r, "mediaID")

	data, metadata, err := h.mediaStore.GetData(mediaID)
	if err != nil {
		h.sendError(w, "Media not found", 404)
		return
	}

	w.Header().Set("Content-Type", metadata.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", mediaID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

func (h *MediaHandler) UploadFromURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL              string `json:"url"`
		MessagingProduct string `json:"messaging_product"`
		Type             string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid JSON", 400)
		return
	}

	resp, err := http.Get(req.URL)
	if err != nil {
		h.sendError(w, "Failed to fetch URL", 500)
		return
	}
	defer resp.Body.Close()

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = req.Type
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		h.sendError(w, "Failed to read response", 500)
		return
	}

	mediaID, err := h.mediaStore.Store(data, mimeType, getMediaType(mimeType))
	if err != nil {
		h.sendError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id": mediaID,
	})
}

func (h *MediaHandler) sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "OAuthException",
			"code":    code,
		},
	})
}

func getMediaType(mimeType string) string {
	if strings.HasPrefix(mimeType, "image/") {
		return "image"
	}
	if strings.HasPrefix(mimeType, "video/") {
		return "video"
	}
	if strings.HasPrefix(mimeType, "audio/") {
		return "audio"
	}
	return "document"
}

func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/shridarpatil/bifrost/store"
	"github.com/shridarpatil/bifrost/translator"
	"github.com/shridarpatil/bifrost/whatsapp"
)

type MessageHandler struct {
	waClient   *whatsapp.Client
	mediaStore *store.MediaStore
}

func NewMessageHandler(waClient *whatsapp.Client, mediaStore *store.MediaStore) *MessageHandler {
	return &MessageHandler{
		waClient:   waClient,
		mediaStore: mediaStore,
	}
}

func (h *MessageHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v{version}/{phoneID}/messages", h.SendMessage)
}

func (h *MessageHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	phoneID := chi.URLParam(r, "phoneID")

	if !h.waClient.IsConnected(phoneID) {
		h.sendError(w, "WhatsApp not connected", 400)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendError(w, "Failed to read request body", 400)
		return
	}

	var req translator.CloudAPIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendError(w, "Invalid JSON", 400)
		return
	}

	if err := req.Validate(); err != nil {
		h.sendError(w, err.Error(), 400)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	recipient := translator.NormalizePhoneNumber(req.To)
	var messageID string

	// Extract reply-to message ID if present
	var replyToMsgID string
	if req.Context != nil && req.Context.MessageID != "" {
		replyToMsgID = req.Context.MessageID
	}

	switch req.Type {
	case "text":
		messageID, err = h.waClient.SendTextMessageWithContext(ctx, phoneID, recipient, req.Text.Body, replyToMsgID)

	case "image":
		messageID, err = h.sendMediaMessage(ctx, phoneID, recipient, req.Image, "image", req)

	case "video":
		messageID, err = h.sendMediaMessage(ctx, phoneID, recipient, req.Video, "video", req)

	case "audio":
		messageID, err = h.sendMediaMessage(ctx, phoneID, recipient, req.Audio, "audio", req)

	case "document":
		messageID, err = h.sendMediaMessage(ctx, phoneID, recipient, req.Document, "document", req)

	case "interactive":
		messageID, err = h.sendInteractiveMessage(ctx, phoneID, recipient, req.Interactive)

	default:
		h.sendError(w, "Unsupported message type", 400)
		return
	}

	if err != nil {
		h.sendError(w, err.Error(), 500)
		return
	}

	response := translator.CreateSuccessResponse(req.To, messageID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *MessageHandler) sendMediaMessage(ctx context.Context, phoneID, recipient string, media *translator.MediaObject, mediaType string, req translator.CloudAPIRequest) (string, error) {
	var data []byte
	var mimeType string
	var err error

	if media.ID != "" {
		data, mimeType, err = h.getMediaByID(media.ID)
	} else if media.Link != "" {
		data, mimeType, err = h.downloadMedia(ctx, media.Link)
	}

	if err != nil {
		return "", err
	}

	switch mediaType {
	case "image":
		return h.waClient.SendImageMessage(ctx, phoneID, recipient, data, mimeType, media.Caption)
	case "video":
		return h.waClient.SendVideoMessage(ctx, phoneID, recipient, data, mimeType, media.Caption)
	case "audio":
		return h.waClient.SendAudioMessage(ctx, phoneID, recipient, data, mimeType)
	case "document":
		return h.waClient.SendDocumentMessage(ctx, phoneID, recipient, data, mimeType, media.Filename, media.Caption)
	}

	return "", nil
}

func (h *MessageHandler) getMediaByID(id string) ([]byte, string, error) {
	data, metadata, err := h.mediaStore.GetData(id)
	if err != nil {
		return nil, "", err
	}
	return data, metadata.MimeType, nil
}

func (h *MessageHandler) downloadMedia(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return data, mimeType, nil
}

func (h *MessageHandler) sendInteractiveMessage(ctx context.Context, phoneID, recipient string, interactive *translator.Interactive) (string, error) {
	if interactive.Type == "button" {
		var buttons []whatsapp.Button
		for _, btn := range interactive.Action.Buttons {
			buttons = append(buttons, whatsapp.Button{
				ID:    btn.Reply.ID,
				Title: btn.Reply.Title,
			})
		}

		footer := ""
		if interactive.Footer != nil {
			footer = interactive.Footer.Text
		}

		return h.waClient.SendInteractiveButtons(ctx, phoneID, recipient, interactive.Body.Text, footer, buttons)
	}

	return "", nil
}

func (h *MessageHandler) sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(translator.CreateErrorResponse(message, code))
}

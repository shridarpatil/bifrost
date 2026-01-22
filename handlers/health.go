package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shridarpatil/bifrost/whatsapp"
)

type HealthHandler struct {
	sessionMgr *whatsapp.SessionManager
}

func NewHealthHandler(sessionMgr *whatsapp.SessionManager) *HealthHandler {
	return &HealthHandler{sessionMgr: sessionMgr}
}

func (h *HealthHandler) RegisterRoutes(r chi.Router) {
	r.Get("/health", h.Health)
	r.Get("/", h.Health)
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"service": "wa-cloud-proxy",
	})
}

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"

	"github.com/shridarpatil/bifrost/config"
	"github.com/shridarpatil/bifrost/whatsapp"
)

type AuthHandler struct {
	sessionMgr *whatsapp.SessionManager
	cfg        *config.Config
}

func NewAuthHandler(sessionMgr *whatsapp.SessionManager, cfg *config.Config) *AuthHandler {
	return &AuthHandler{sessionMgr: sessionMgr, cfg: cfg}
}

func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Get("/auth/qr/{phoneID}", h.QRCode)
	r.Get("/auth/status/{phoneID}", h.Status)
	r.Post("/auth/logout/{phoneID}", h.Logout)
}

func (h *AuthHandler) QRCode(w http.ResponseWriter, r *http.Request) {
	phoneID := chi.URLParam(r, "phoneID")
	format := r.URL.Query().Get("format")

	status := h.sessionMgr.GetStatus(phoneID)
	if status.LoggedIn {
		if format == "json" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "connected",
				"phone":  status.PhoneNumber,
				"name":   status.PushName,
			})
			return
		}
		h.renderStatusPage(w, phoneID, "connected", status.PushName)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	qrChan, err := h.sessionMgr.StartQRLogin(ctx, phoneID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if format == "json" {
		h.handleJSONQR(w, r, qrChan)
		return
	}

	h.handleHTMLQR(w, r, phoneID, qrChan)
}

func (h *AuthHandler) handleJSONQR(w http.ResponseWriter, r *http.Request, qrChan <-chan whatsapp.QREvent) {
	select {
	case evt, ok := <-qrChan:
		if !ok {
			json.NewEncoder(w).Encode(map[string]string{"error": "channel closed"})
			return
		}

		if evt.Error != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": evt.Error.Error()})
			return
		}

		if evt.Code == "" {
			json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
			return
		}

		png, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"qr_code":    base64.StdEncoding.EncodeToString(png),
			"expires_in": int(time.Until(evt.ExpiresAt).Seconds()),
		})

	case <-r.Context().Done():
		json.NewEncoder(w).Encode(map[string]string{"error": "timeout"})
	}
}

func (h *AuthHandler) handleHTMLQR(w http.ResponseWriter, r *http.Request, phoneID string, qrChan <-chan whatsapp.QREvent) {
	select {
	case evt, ok := <-qrChan:
		if !ok {
			h.renderStatusPage(w, phoneID, "error", "Connection closed")
			return
		}

		if evt.Error != nil {
			if evt.Error.Error() == "already logged in" {
				status := h.sessionMgr.GetStatus(phoneID)
				h.renderStatusPage(w, phoneID, "connected", status.PushName)
				return
			}
			h.renderStatusPage(w, phoneID, "error", evt.Error.Error())
			return
		}

		if evt.Code == "" {
			status := h.sessionMgr.GetStatus(phoneID)
			h.renderStatusPage(w, phoneID, "connected", status.PushName)
			return
		}

		png, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		h.renderQRPage(w, phoneID, base64.StdEncoding.EncodeToString(png), int(time.Until(evt.ExpiresAt).Seconds()))

	case <-r.Context().Done():
		h.renderStatusPage(w, phoneID, "error", "Timeout waiting for QR code")
	}
}

func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	phoneID := chi.URLParam(r, "phoneID")
	status := h.sessionMgr.GetStatus(phoneID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	phoneID := chi.URLParam(r, "phoneID")

	if err := h.sessionMgr.Logout(phoneID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
}

func (h *AuthHandler) getInstancePhone(phoneID string) string {
	if inst := h.cfg.GetInstance(phoneID); inst != nil && inst.Phone != "" {
		return inst.Phone
	}
	return phoneID
}

func (h *AuthHandler) renderQRPage(w http.ResponseWriter, phoneID, qrBase64 string, expiresIn int) {
	tmpl := template.Must(template.New("qr").Parse(qrPageTemplate))
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, map[string]interface{}{
		"PhoneID":     phoneID,
		"PhoneNumber": h.getInstancePhone(phoneID),
		"QRCode":      qrBase64,
		"ExpiresIn":   expiresIn,
	})
}

func (h *AuthHandler) renderStatusPage(w http.ResponseWriter, phoneID, status, message string) {
	tmpl := template.Must(template.New("status").Parse(statusPageTemplate))
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, map[string]interface{}{
		"PhoneID":     phoneID,
		"PhoneNumber": h.getInstancePhone(phoneID),
		"Status":      status,
		"Message":     message,
	})
}

const qrPageTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>WhatsApp QR Code - {{.PhoneID}}</title>
    <meta http-equiv="refresh" content="{{.ExpiresIn}}">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        .container {
            background: white;
            padding: 40px;
            border-radius: 16px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            text-align: center;
        }
        h1 {
            margin: 0 0 10px 0;
            color: #333;
            font-size: 24px;
        }
        .subtitle {
            color: #666;
            margin-bottom: 30px;
        }
        .qr-code {
            margin: 20px 0;
        }
        .qr-code img {
            border: 4px solid #25D366;
            border-radius: 8px;
        }
        .instructions {
            color: #666;
            font-size: 14px;
            line-height: 1.6;
            max-width: 300px;
            margin: 20px auto 0;
        }
        .timer {
            color: #999;
            font-size: 12px;
            margin-top: 20px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Link WhatsApp</h1>
        <p class="subtitle">{{.PhoneNumber}}</p>
        <div class="qr-code">
            <img src="data:image/png;base64,{{.QRCode}}" alt="QR Code" width="256" height="256">
        </div>
        <div class="instructions">
            <strong>To link your WhatsApp:</strong><br>
            1. Open WhatsApp on your phone<br>
            2. Tap Menu ⋮ or Settings ⚙️<br>
            3. Tap Linked Devices<br>
            4. Tap Link a Device<br>
            5. Scan this QR code
        </div>
        <p class="timer">Page refreshes automatically when QR expires</p>
    </div>
</body>
</html>`

const statusPageTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>WhatsApp Status - {{.PhoneID}}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        .container {
            background: white;
            padding: 40px;
            border-radius: 16px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            text-align: center;
        }
        h1 {
            margin: 0 0 10px 0;
            font-size: 24px;
        }
        .connected { color: #25D366; }
        .error { color: #e74c3c; }
        .message {
            color: #666;
            margin-top: 20px;
        }
        .icon {
            font-size: 64px;
            margin-bottom: 20px;
        }
    </style>
</head>
<body>
    <div class="container">
        {{if eq .Status "connected"}}
        <div class="icon">✓</div>
        <h1 class="connected">Connected!</h1>
        {{else}}
        <div class="icon">✗</div>
        <h1 class="error">{{.Status}}</h1>
        {{end}}
        <p class="subtitle">{{.PhoneNumber}}</p>
        {{if .Message}}
        <p class="message">{{.Message}}</p>
        {{end}}
    </div>
</body>
</html>`

package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
)

type SessionManager struct {
	client     *Client
	qrChannels map[string]chan QREvent
	qrMu       sync.RWMutex
}

type QREvent struct {
	Code      string
	ExpiresAt time.Time
	Error     error
}

type SessionStatus struct {
	Connected   bool   `json:"connected"`
	LoggedIn    bool   `json:"logged_in"`
	PhoneNumber string `json:"phone_number,omitempty"`
	PushName    string `json:"push_name,omitempty"`
}

func NewSessionManager(client *Client) *SessionManager {
	return &SessionManager{
		client:     client,
		qrChannels: make(map[string]chan QREvent),
	}
}

func (sm *SessionManager) GetStatus(phoneID string) SessionStatus {
	client := sm.client.GetClient(phoneID)
	if client == nil {
		return SessionStatus{Connected: false, LoggedIn: false}
	}

	status := SessionStatus{
		Connected: client.IsConnected(),
		LoggedIn:  client.IsLoggedIn(),
	}

	if client.Store.ID != nil {
		status.PhoneNumber = client.Store.ID.User
		if client.Store.PushName != "" {
			status.PushName = client.Store.PushName
		}
	}

	return status
}

func (sm *SessionManager) StartQRLogin(ctx context.Context, phoneID string) (<-chan QREvent, error) {
	waClient, err := sm.client.GetOrCreateSession(phoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if waClient.Store.ID != nil {
		if err := waClient.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect: %w", err)
		}
		ch := make(chan QREvent, 1)
		ch <- QREvent{Error: fmt.Errorf("already logged in")}
		close(ch)
		return ch, nil
	}

	sm.qrMu.Lock()
	if existing, ok := sm.qrChannels[phoneID]; ok {
		// Don't close existing - reuse the channel to get current QR
		sm.qrMu.Unlock()
		return existing, nil
	}
	qrChan := make(chan QREvent, 10)
	sm.qrChannels[phoneID] = qrChan
	sm.qrMu.Unlock()

	// Use background context so connection stays alive after HTTP request completes
	go sm.handleQRLogin(context.Background(), phoneID, waClient, qrChan)

	return qrChan, nil
}

func (sm *SessionManager) handleQRLogin(ctx context.Context, phoneID string, client *whatsmeow.Client, qrChan chan QREvent) {
	defer func() {
		sm.qrMu.Lock()
		delete(sm.qrChannels, phoneID)
		sm.qrMu.Unlock()
		close(qrChan)
	}()

	internalQR, _ := client.GetQRChannel(ctx)
	err := client.Connect()
	if err != nil {
		qrChan <- QREvent{Error: fmt.Errorf("failed to connect: %w", err)}
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-internalQR:
			if !ok {
				return
			}

			switch evt.Event {
			case "code":
				qrChan <- QREvent{
					Code:      evt.Code,
					ExpiresAt: time.Now().Add(evt.Timeout),
				}
			case "success":
				qrChan <- QREvent{}
				return
			case "timeout":
				qrChan <- QREvent{Error: fmt.Errorf("QR code timeout")}
			}
		}
	}
}

func (sm *SessionManager) Logout(phoneID string) error {
	return sm.client.Logout(phoneID)
}

func (sm *SessionManager) Connect(phoneID string) error {
	return sm.client.Connect(phoneID)
}

func (sm *SessionManager) ConnectAll() {
	for phoneID := range sm.client.cfg.Instances {
		if err := sm.Connect(phoneID); err != nil {
			fmt.Printf("Failed to connect instance %s: %v\n", phoneID, err)
		}
	}
}

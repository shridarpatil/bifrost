package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/shridarpatil/bifrost/config"
)

type Client struct {
	cfg        *config.Config
	container  *sqlstore.Container
	clients    map[string]*whatsmeow.Client
	clientsMu  sync.RWMutex
	eventHandler EventHandler
}

type EventHandler interface {
	HandleEvent(phoneID string, evt interface{})
}

func NewClient(cfg *config.Config) (*Client, error) {
	dbLog := waLog.Stdout("Database", "WARN", true)
	container, err := sqlstore.New(context.Background(), "sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", cfg.WhatsApp.StorePath), dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	return &Client{
		cfg:       cfg,
		container: container,
		clients:   make(map[string]*whatsmeow.Client),
	}, nil
}

func (c *Client) SetEventHandler(handler EventHandler) {
	c.eventHandler = handler
}

func (c *Client) GetOrCreateSession(phoneID string) (*whatsmeow.Client, error) {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()

	if client, ok := c.clients[phoneID]; ok {
		return client, nil
	}

	deviceStore, err := c.getDeviceStore(phoneID)
	if err != nil {
		return nil, err
	}

	clientLog := waLog.Stdout("Client", "WARN", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	client.AddEventHandler(func(evt interface{}) {
		if c.eventHandler != nil {
			c.eventHandler.HandleEvent(phoneID, evt)
		}
	})

	c.clients[phoneID] = client
	return client, nil
}

func (c *Client) getDeviceStore(phoneID string) (*store.Device, error) {
	devices, err := c.container.GetAllDevices(context.Background())
	if err != nil {
		return nil, err
	}

	for _, device := range devices {
		if device.ID != nil && device.ID.String() != "" {
			return device, nil
		}
	}

	return c.container.NewDevice(), nil
}

func (c *Client) GetClient(phoneID string) *whatsmeow.Client {
	c.clientsMu.RLock()
	defer c.clientsMu.RUnlock()
	return c.clients[phoneID]
}

func (c *Client) IsConnected(phoneID string) bool {
	client := c.GetClient(phoneID)
	if client == nil {
		return false
	}
	return client.IsConnected() && client.IsLoggedIn()
}

func (c *Client) Connect(phoneID string) error {
	client, err := c.GetOrCreateSession(phoneID)
	if err != nil {
		return err
	}

	if client.Store.ID != nil {
		return client.Connect()
	}

	return nil
}

func (c *Client) Disconnect(phoneID string) {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()

	if client, ok := c.clients[phoneID]; ok {
		client.Disconnect()
		delete(c.clients, phoneID)
	}
}

func (c *Client) Logout(phoneID string) error {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()

	if client, ok := c.clients[phoneID]; ok {
		err := client.Logout(context.Background())
		client.Disconnect()
		delete(c.clients, phoneID)
		return err
	}
	return nil
}

func (c *Client) SendTextMessage(ctx context.Context, phoneID, recipient, text string) (string, error) {
	return c.SendTextMessageWithContext(ctx, phoneID, recipient, text, "")
}

func (c *Client) SendTextMessageWithContext(ctx context.Context, phoneID, recipient, text, replyToMsgID string) (string, error) {
	client := c.GetClient(phoneID)
	if client == nil {
		return "", fmt.Errorf("no client for phone_id: %s", phoneID)
	}

	// Detect if recipient is a group JID (contains @g.us)
	var jid types.JID
	var err error
	if strings.Contains(recipient, "@g.us") {
		jid, err = types.ParseJID(recipient)
	} else {
		jid, err = types.ParseJID(recipient + "@s.whatsapp.net")
	}
	if err != nil {
		return "", fmt.Errorf("invalid recipient: %w", err)
	}

	var msg *waE2E.Message
	if replyToMsgID != "" {
		// Send as reply with context info
		msg = &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(text),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    proto.String(replyToMsgID),
					Participant: proto.String(recipient + "@s.whatsapp.net"),
				},
			},
		}
	} else {
		// Regular message
		msg = &waE2E.Message{
			Conversation: proto.String(text),
		}
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (c *Client) SendImageMessage(ctx context.Context, phoneID, recipient string, data []byte, mimeType, caption string) (string, error) {
	client := c.GetClient(phoneID)
	if client == nil {
		return "", fmt.Errorf("no client for phone_id: %s", phoneID)
	}

	jid, err := types.ParseJID(recipient + "@s.whatsapp.net")
	if err != nil {
		return "", fmt.Errorf("invalid recipient: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("failed to upload image: %w", err)
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Caption:       proto.String(caption),
		},
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (c *Client) SendVideoMessage(ctx context.Context, phoneID, recipient string, data []byte, mimeType, caption string) (string, error) {
	client := c.GetClient(phoneID)
	if client == nil {
		return "", fmt.Errorf("no client for phone_id: %s", phoneID)
	}

	jid, err := types.ParseJID(recipient + "@s.whatsapp.net")
	if err != nil {
		return "", fmt.Errorf("invalid recipient: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return "", fmt.Errorf("failed to upload video: %w", err)
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Caption:       proto.String(caption),
		},
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (c *Client) SendAudioMessage(ctx context.Context, phoneID, recipient string, data []byte, mimeType string) (string, error) {
	client := c.GetClient(phoneID)
	if client == nil {
		return "", fmt.Errorf("no client for phone_id: %s", phoneID)
	}

	jid, err := types.ParseJID(recipient + "@s.whatsapp.net")
	if err != nil {
		return "", fmt.Errorf("invalid recipient: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("failed to upload audio: %w", err)
	}

	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (c *Client) SendDocumentMessage(ctx context.Context, phoneID, recipient string, data []byte, mimeType, filename, caption string) (string, error) {
	client := c.GetClient(phoneID)
	if client == nil {
		return "", fmt.Errorf("no client for phone_id: %s", phoneID)
	}

	jid, err := types.ParseJID(recipient + "@s.whatsapp.net")
	if err != nil {
		return "", fmt.Errorf("invalid recipient: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return "", fmt.Errorf("failed to upload document: %w", err)
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileName:      proto.String(filename),
			Caption:       proto.String(caption),
		},
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (c *Client) SendInteractiveButtons(ctx context.Context, phoneID, recipient, body, footer string, buttons []Button) (string, error) {
	client := c.GetClient(phoneID)
	if client == nil {
		return "", fmt.Errorf("no client for phone_id: %s", phoneID)
	}

	jid, err := types.ParseJID(recipient + "@s.whatsapp.net")
	if err != nil {
		return "", fmt.Errorf("invalid recipient: %w", err)
	}

	var waButtons []*waE2E.ButtonsMessage_Button
	for i, btn := range buttons {
		waButtons = append(waButtons, &waE2E.ButtonsMessage_Button{
			ButtonID: proto.String(btn.ID),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: proto.String(btn.Title),
			},
			Type:        waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			NativeFlowInfo: nil,
		})
		if i >= 2 {
			break
		}
	}

	msg := &waE2E.Message{
		ButtonsMessage: &waE2E.ButtonsMessage{
			ContentText: proto.String(body),
			FooterText:  proto.String(footer),
			Buttons:     waButtons,
			HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
		},
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (c *Client) MarkAsRead(ctx context.Context, phoneID string, messageIDs []string, sender string) error {
	client := c.GetClient(phoneID)
	if client == nil {
		return fmt.Errorf("no client for phone_id: %s", phoneID)
	}

	jid, err := types.ParseJID(sender + "@s.whatsapp.net")
	if err != nil {
		return fmt.Errorf("invalid sender: %w", err)
	}

	msgIDs := make([]types.MessageID, len(messageIDs))
	for i, id := range messageIDs {
		msgIDs[i] = types.MessageID(id)
	}

	return client.MarkRead(ctx, msgIDs, time.Now(), jid, jid)
}

func (c *Client) Close() {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()

	for _, client := range c.clients {
		client.Disconnect()
	}
	c.clients = make(map[string]*whatsmeow.Client)
}

type Button struct {
	ID    string
	Title string
}

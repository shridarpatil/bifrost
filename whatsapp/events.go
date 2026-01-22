package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/shridarpatil/bifrost/config"
	"github.com/shridarpatil/bifrost/store"
	"github.com/shridarpatil/bifrost/translator"
)

type EventProcessor struct {
	cfg        *config.Config
	waClient   *Client
	mediaStore *store.MediaStore
	httpClient *http.Client
}

func NewEventProcessor(cfg *config.Config, waClient *Client, mediaStore *store.MediaStore) *EventProcessor {
	return &EventProcessor{
		cfg:        cfg,
		waClient:   waClient,
		mediaStore: mediaStore,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (ep *EventProcessor) HandleEvent(phoneID string, evt interface{}) {
	switch e := evt.(type) {
	case *events.Message:
		ep.handleMessage(phoneID, e)
	case *events.Receipt:
		log.Printf("[%s] DEBUG Receipt received: Type=%v, Chat=%s, MessageIDs=%v", phoneID, e.Type, e.Chat.String(), e.MessageIDs)
		ep.handleReceipt(phoneID, e)
	case *events.Connected:
		log.Printf("[%s] Connected to WhatsApp", phoneID)
	case *events.Disconnected:
		log.Printf("[%s] Disconnected from WhatsApp", phoneID)
	case *events.LoggedOut:
		log.Printf("[%s] Logged out", phoneID)
	}
}

func (ep *EventProcessor) handleMessage(phoneID string, msg *events.Message) {
	// Debug: log all message info
	log.Printf("[%s] DEBUG Message: IsFromMe=%v, Type=%s, ID=%s, Chat=%s",
		phoneID, msg.Info.IsFromMe, msg.Info.Type, msg.Info.ID, msg.Info.Chat.Server)

	if msg.Info.IsFromMe {
		return
	}

	// For group messages, we'll treat the group as a "contact"
	// The group JID will be used as the phone number

	client := ep.waClient.GetClient(phoneID)
	if client == nil {
		log.Printf("[%s] No client found for message", phoneID)
		return
	}

	webhook, err := ep.translateMessage(phoneID, msg, client)
	if err != nil {
		log.Printf("[%s] Failed to translate message: %v", phoneID, err)
		return
	}
	if webhook == nil {
		// Unknown message type, already logged
		return
	}

	ep.sendWebhook(webhook)
}

func (ep *EventProcessor) translateMessage(phoneID string, msg *events.Message, client *whatsmeow.Client) (*translator.CloudAPIWebhook, error) {
	ctx := context.Background()

	// Debug: Log all JID values to understand where LID comes from
	log.Printf("[%s] DEBUG JIDs - Chat: %s (Server: %s), Sender: %s (Server: %s), SenderAlt: %s",
		phoneID,
		msg.Info.Chat.User, msg.Info.Chat.Server,
		msg.Info.Sender.User, msg.Info.Sender.Server,
		msg.Info.SenderAlt.String())

	var sender string

	// Handle LID (Linked ID) - WhatsApp's privacy feature
	// When server is "lid", we need to resolve to actual phone number
	if msg.Info.Chat.Server == "lid" {
		// First check if SenderAlt has the phone number (when sender is LID, alt is often PN)
		if !msg.Info.SenderAlt.IsEmpty() && msg.Info.SenderAlt.Server == "s.whatsapp.net" {
			sender = msg.Info.SenderAlt.User
			log.Printf("[%s] DEBUG Using SenderAlt phone: %s", phoneID, sender)
		} else {
			// Try to resolve LID to PN using whatmeow's LID store
			pnJID, err := client.Store.LIDs.GetPNForLID(ctx, msg.Info.Chat)
			if err == nil && !pnJID.IsEmpty() {
				sender = pnJID.User
				log.Printf("[%s] DEBUG Resolved LID to phone via store: %s", phoneID, sender)
			} else {
				// Fallback: use the LID but log warning
				sender = msg.Info.Chat.User
				log.Printf("[%s] WARNING: Could not resolve LID %s to phone number (err: %v)", phoneID, sender, err)
			}
		}
	} else if msg.Info.Chat.Server == "g.us" {
		// For group messages, use the GROUP JID as the "sender" (contact)
		// This treats the group as a contact in Whatomate
		sender = msg.Info.Chat.User + "@g.us"
		log.Printf("[%s] DEBUG Group message: using group JID as sender: %s", phoneID, sender)
	} else {
		// Regular s.whatsapp.net chat - use Chat JID
		sender = msg.Info.Chat.User
	}

	log.Printf("[%s] DEBUG Using sender: %s", phoneID, sender)
	timestamp := msg.Info.Timestamp.Unix()

	// Get profile name - for groups, use group name
	// Also track the actual sender within the group
	profileName := msg.Info.PushName
	var groupSenderPrefix string
	if msg.Info.Chat.Server == "g.us" {
		groupInfo, err := client.GetGroupInfo(ctx, msg.Info.Chat)
		if err == nil && groupInfo != nil {
			profileName = groupInfo.Name
		} else {
			profileName = "Group " + msg.Info.Chat.User
		}
		// Get the actual sender's name within the group
		actualSenderName := msg.Info.PushName
		if actualSenderName == "" {
			actualSenderName = msg.Info.Sender.User
		}
		groupSenderPrefix = actualSenderName + ": "
		log.Printf("[%s] Group message in '%s' from %s (%s)", phoneID, profileName, actualSenderName, msg.Info.Sender.User)
	}

	webhook := &translator.CloudAPIWebhook{
		Object: "whatsapp_business_account",
		Entry: []translator.Entry{
			{
				ID: phoneID,
				Changes: []translator.Change{
					{
						Value: translator.ChangeValue{
							MessagingProduct: "whatsapp",
							Metadata: translator.Metadata{
								DisplayPhoneNumber: phoneID,
								PhoneNumberID:      phoneID,
							},
							Contacts: []translator.Contact{
								{
									WaID: sender,
									Profile: translator.Profile{
										Name: profileName,
									},
								},
							},
						},
						Field: "messages",
					},
				},
			},
		},
	}

	cloudMsg := translator.Message{
		From:      sender,
		ID:        msg.Info.ID,
		Timestamp: fmt.Sprintf("%d", timestamp),
	}

	if msg.Message.GetConversation() != "" {
		cloudMsg.Type = "text"
		cloudMsg.Text = &translator.TextContent{
			Body: groupSenderPrefix + msg.Message.GetConversation(),
		}
	} else if msg.Message.GetExtendedTextMessage() != nil {
		cloudMsg.Type = "text"
		cloudMsg.Text = &translator.TextContent{
			Body: groupSenderPrefix + msg.Message.GetExtendedTextMessage().GetText(),
		}
	} else if img := msg.Message.GetImageMessage(); img != nil {
		mediaID, err := ep.downloadAndStoreMedia(context.Background(), client, img, "image")
		if err != nil {
			log.Printf("[%s] Failed to download image: %v", phoneID, err)
		}
		imgCaption := img.GetCaption()
		if groupSenderPrefix != "" {
			imgCaption = groupSenderPrefix + imgCaption
		}
		cloudMsg.Type = "image"
		cloudMsg.Image = &translator.MediaContent{
			ID:       mediaID,
			MimeType: img.GetMimetype(),
			Caption:  imgCaption,
		}
	} else if vid := msg.Message.GetVideoMessage(); vid != nil {
		mediaID, err := ep.downloadAndStoreMedia(context.Background(), client, vid, "video")
		if err != nil {
			log.Printf("[%s] Failed to download video: %v", phoneID, err)
		}
		vidCaption := vid.GetCaption()
		if groupSenderPrefix != "" {
			vidCaption = groupSenderPrefix + vidCaption
		}
		cloudMsg.Type = "video"
		cloudMsg.Video = &translator.MediaContent{
			ID:       mediaID,
			MimeType: vid.GetMimetype(),
			Caption:  vidCaption,
		}
	} else if aud := msg.Message.GetAudioMessage(); aud != nil {
		mediaID, err := ep.downloadAndStoreMedia(context.Background(), client, aud, "audio")
		if err != nil {
			log.Printf("[%s] Failed to download audio: %v", phoneID, err)
		}
		cloudMsg.Type = "audio"
		cloudMsg.Audio = &translator.MediaContent{
			ID:       mediaID,
			MimeType: aud.GetMimetype(),
			Caption:  groupSenderPrefix, // Audio has no caption, use sender name for groups
		}
	} else if doc := msg.Message.GetDocumentMessage(); doc != nil {
		mediaID, err := ep.downloadAndStoreMedia(context.Background(), client, doc, "document")
		if err != nil {
			log.Printf("[%s] Failed to download document: %v", phoneID, err)
		}
		docCaption := doc.GetCaption()
		if groupSenderPrefix != "" {
			docCaption = groupSenderPrefix + docCaption
		}
		cloudMsg.Type = "document"
		cloudMsg.Document = &translator.MediaContent{
			ID:       mediaID,
			MimeType: doc.GetMimetype(),
			Filename: doc.GetFileName(),
			Caption:  docCaption,
		}
	} else if btnResp := msg.Message.GetButtonsResponseMessage(); btnResp != nil {
		cloudMsg.Type = "interactive"
		cloudMsg.Interactive = &translator.InteractiveContent{
			Type: "button_reply",
			ButtonReply: &translator.ButtonReplyData{
				ID:    btnResp.GetSelectedButtonID(),
				Title: btnResp.GetSelectedDisplayText(),
			},
		}
	} else {
		// Skip unknown/protocol messages (receipts, presence, etc.)
		log.Printf("[%s] Skipping unknown message type from %s", phoneID, sender)
		return nil, nil
	}

	// Extract context info (reply-to) if present
	contextInfo := ep.extractContextInfo(msg.Message)
	if contextInfo != nil && contextInfo.GetStanzaID() != "" {
		// Resolve participant (who sent the original message)
		replyFrom := contextInfo.GetParticipant()
		if replyFrom == "" {
			// For 1-on-1 chats, use sender as participant
			replyFrom = sender
		}
		cloudMsg.Context = &translator.MessageContext{
			From: replyFrom,
			ID:   contextInfo.GetStanzaID(),
		}
		log.Printf("[%s] Message is a reply to %s", phoneID, contextInfo.GetStanzaID())
	}

	webhook.Entry[0].Changes[0].Value.Messages = []translator.Message{cloudMsg}

	return webhook, nil
}

// extractContextInfo extracts the context info from various message types
func (ep *EventProcessor) extractContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}

	// Check each message type that can have context info
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetContextInfo()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetContextInfo()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetContextInfo()
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return aud.GetContextInfo()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetContextInfo()
	}
	if sticker := msg.GetStickerMessage(); sticker != nil {
		return sticker.GetContextInfo()
	}
	if btnResp := msg.GetButtonsResponseMessage(); btnResp != nil {
		return btnResp.GetContextInfo()
	}

	return nil
}

type downloadableMessage interface {
	GetURL() string
	GetDirectPath() string
	GetMediaKey() []byte
	GetFileEncSHA256() []byte
	GetFileSHA256() []byte
	GetFileLength() uint64
	GetMimetype() string
}

func (ep *EventProcessor) downloadAndStoreMedia(ctx context.Context, client *whatsmeow.Client, msg whatsmeow.DownloadableMessage, mediaType string) (string, error) {
	data, err := client.Download(ctx, msg)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	mimeType := ""
	if m, ok := msg.(interface{ GetMimetype() string }); ok {
		mimeType = m.GetMimetype()
	}

	mediaID, err := ep.mediaStore.Store(data, mimeType, mediaType)
	if err != nil {
		return "", fmt.Errorf("store failed: %w", err)
	}

	return mediaID, nil
}

func (ep *EventProcessor) handleReceipt(phoneID string, receipt *events.Receipt) {
	if ep.cfg.Webhook.URL == "" {
		log.Printf("[%s] Receipt: No webhook URL configured", phoneID)
		return
	}

	// Skip status updates for group chats
	if receipt.Chat.Server == "g.us" {
		log.Printf("[%s] Receipt: Skipping group chat status update", phoneID)
		return
	}

	if len(receipt.MessageIDs) == 0 {
		log.Printf("[%s] Receipt: No message IDs in receipt", phoneID)
		return
	}

	var status string
	switch receipt.Type {
	case events.ReceiptTypeDelivered:
		status = "delivered"
	case events.ReceiptTypeRead:
		status = "read"
	case events.ReceiptTypeSender:
		status = "sent"
	default:
		log.Printf("[%s] Receipt: Unknown receipt type %v, skipping", phoneID, receipt.Type)
		return
	}
	log.Printf("[%s] Receipt: Processing %s for message %s", phoneID, status, receipt.MessageIDs[0])

	// Resolve LID to phone number if needed
	client := ep.waClient.GetClient(phoneID)
	recipientID := receipt.Chat.User
	if receipt.Chat.Server == "lid" && client != nil {
		ctx := context.Background()
		pnJID, err := client.Store.LIDs.GetPNForLID(ctx, receipt.Chat)
		if err == nil && !pnJID.IsEmpty() {
			recipientID = pnJID.User
			log.Printf("[%s] Receipt: Resolved LID to phone: %s", phoneID, recipientID)
		} else {
			log.Printf("[%s] Receipt: Could not resolve LID %s to phone", phoneID, recipientID)
		}
	}

	webhook := &translator.CloudAPIWebhook{
		Object: "whatsapp_business_account",
		Entry: []translator.Entry{
			{
				ID: phoneID,
				Changes: []translator.Change{
					{
						Value: translator.ChangeValue{
							MessagingProduct: "whatsapp",
							Metadata: translator.Metadata{
								DisplayPhoneNumber: phoneID,
								PhoneNumberID:      phoneID,
							},
							Statuses: []translator.Status{
								{
									ID:          receipt.MessageIDs[0],
									Status:      status,
									Timestamp:   fmt.Sprintf("%d", receipt.Timestamp.Unix()),
									RecipientID: recipientID,
								},
							},
						},
						Field: "messages",
					},
				},
			},
		},
	}

	log.Printf("[%s] Receipt: Sending webhook for %s status", phoneID, status)
	ep.sendWebhook(webhook)
}

func (ep *EventProcessor) sendWebhook(webhook *translator.CloudAPIWebhook) {
	if ep.cfg.Webhook.URL == "" {
		return
	}

	data, err := json.Marshal(webhook)
	if err != nil {
		log.Printf("Failed to marshal webhook: %v", err)
		return
	}

	log.Printf("Sending webhook to %s: %s", ep.cfg.Webhook.URL, string(data))

	req, err := http.NewRequest("POST", ep.cfg.Webhook.URL, bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to create webhook request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if ep.cfg.Webhook.Secret != "" {
		req.Header.Set("X-Hub-Signature-256", ep.cfg.Webhook.Secret)
	}

	resp, err := ep.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to send webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("Webhook returned error status: %d", resp.StatusCode)
	} else {
		log.Printf("Webhook sent successfully, status: %d", resp.StatusCode)
	}
}

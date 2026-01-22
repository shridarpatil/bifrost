package translator

import (
	"fmt"
	"strings"
)

// CloudAPIRequest represents the incoming Cloud API message request
type CloudAPIRequest struct {
	MessagingProduct string       `json:"messaging_product"`
	RecipientType    string       `json:"recipient_type,omitempty"`
	To               string       `json:"to"`
	Type             string       `json:"type"`
	Text             *TextMessage `json:"text,omitempty"`
	Image            *MediaObject `json:"image,omitempty"`
	Video            *MediaObject `json:"video,omitempty"`
	Audio            *MediaObject `json:"audio,omitempty"`
	Document         *MediaObject `json:"document,omitempty"`
	Interactive      *Interactive `json:"interactive,omitempty"`
	Context          *Context     `json:"context,omitempty"`
}

type TextMessage struct {
	Body         string `json:"body"`
	PreviewURL   bool   `json:"preview_url,omitempty"`
}

type MediaObject struct {
	ID       string `json:"id,omitempty"`
	Link     string `json:"link,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type Interactive struct {
	Type   string          `json:"type"`
	Header *InteractiveHeader `json:"header,omitempty"`
	Body   *InteractiveBody   `json:"body"`
	Footer *InteractiveFooter `json:"footer,omitempty"`
	Action *InteractiveAction `json:"action"`
}

type InteractiveHeader struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	Image    *MediaObject `json:"image,omitempty"`
	Video    *MediaObject `json:"video,omitempty"`
	Document *MediaObject `json:"document,omitempty"`
}

type InteractiveBody struct {
	Text string `json:"text"`
}

type InteractiveFooter struct {
	Text string `json:"text"`
}

type InteractiveAction struct {
	Buttons []ActionButton `json:"buttons,omitempty"`
	Button  string         `json:"button,omitempty"`
	Sections []Section     `json:"sections,omitempty"`
}

type ActionButton struct {
	Type  string       `json:"type"`
	Reply *ButtonReply `json:"reply"`
}

type ButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Section struct {
	Title string       `json:"title,omitempty"`
	Rows  []SectionRow `json:"rows"`
}

type SectionRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type Context struct {
	MessageID string `json:"message_id"`
}

// CloudAPIResponse represents the response to send back to the client
type CloudAPIResponse struct {
	MessagingProduct string           `json:"messaging_product"`
	Contacts         []ResponseContact `json:"contacts"`
	Messages         []ResponseMessage `json:"messages"`
}

type ResponseContact struct {
	Input string `json:"input"`
	WaID  string `json:"wa_id"`
}

type ResponseMessage struct {
	ID string `json:"id"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Code      int    `json:"code"`
	FBTraceID string `json:"fbtrace_id,omitempty"`
}

// NormalizePhoneNumber ensures the phone number is in the correct format
func NormalizePhoneNumber(phone string) string {
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	phone = strings.TrimPrefix(phone, "+")
	return phone
}

// Validate checks if the request is valid
func (r *CloudAPIRequest) Validate() error {
	if r.MessagingProduct != "whatsapp" {
		return fmt.Errorf("invalid messaging_product: %s", r.MessagingProduct)
	}

	if r.To == "" {
		return fmt.Errorf("recipient 'to' is required")
	}

	switch r.Type {
	case "text":
		if r.Text == nil || r.Text.Body == "" {
			return fmt.Errorf("text body is required for text messages")
		}
	case "image", "video", "audio", "document":
		media := r.GetMedia()
		if media == nil || (media.ID == "" && media.Link == "") {
			return fmt.Errorf("media id or link is required for %s messages", r.Type)
		}
	case "interactive":
		if r.Interactive == nil {
			return fmt.Errorf("interactive object is required for interactive messages")
		}
	default:
		return fmt.Errorf("unsupported message type: %s", r.Type)
	}

	return nil
}

func (r *CloudAPIRequest) GetMedia() *MediaObject {
	switch r.Type {
	case "image":
		return r.Image
	case "video":
		return r.Video
	case "audio":
		return r.Audio
	case "document":
		return r.Document
	}
	return nil
}

// CreateSuccessResponse creates a success response
func CreateSuccessResponse(to, messageID string) *CloudAPIResponse {
	normalizedTo := NormalizePhoneNumber(to)
	return &CloudAPIResponse{
		MessagingProduct: "whatsapp",
		Contacts: []ResponseContact{
			{Input: to, WaID: normalizedTo},
		},
		Messages: []ResponseMessage{
			{ID: messageID},
		},
	}
}

// CreateErrorResponse creates an error response
func CreateErrorResponse(message string, code int) *ErrorResponse {
	return &ErrorResponse{
		Error: ErrorDetail{
			Message: message,
			Type:    "OAuthException",
			Code:    code,
		},
	}
}

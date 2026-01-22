package translator

// CloudAPIWebhook represents the webhook payload sent to the client
type CloudAPIWebhook struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

type Entry struct {
	ID      string   `json:"id"`
	Changes []Change `json:"changes"`
}

type Change struct {
	Value ChangeValue `json:"value"`
	Field string      `json:"field"`
}

type ChangeValue struct {
	MessagingProduct string    `json:"messaging_product"`
	Metadata         Metadata  `json:"metadata"`
	Contacts         []Contact `json:"contacts,omitempty"`
	Messages         []Message `json:"messages,omitempty"`
	Statuses         []Status  `json:"statuses,omitempty"`
}

type Metadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type Contact struct {
	WaID    string  `json:"wa_id"`
	Profile Profile `json:"profile"`
}

type Profile struct {
	Name string `json:"name"`
}

type Message struct {
	From        string              `json:"from"`
	ID          string              `json:"id"`
	Timestamp   string              `json:"timestamp"`
	Type        string              `json:"type"`
	Text        *TextContent        `json:"text,omitempty"`
	Image       *MediaContent       `json:"image,omitempty"`
	Video       *MediaContent       `json:"video,omitempty"`
	Audio       *MediaContent       `json:"audio,omitempty"`
	Document    *MediaContent       `json:"document,omitempty"`
	Interactive *InteractiveContent `json:"interactive,omitempty"`
	Context     *MessageContext     `json:"context,omitempty"`
	// Group message fields (custom extension, not part of Cloud API)
	GroupID   string `json:"group_id,omitempty"`
	GroupName string `json:"group_name,omitempty"`
}

type TextContent struct {
	Body string `json:"body"`
}

type MediaContent struct {
	ID       string `json:"id,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type InteractiveContent struct {
	Type        string           `json:"type"`
	ButtonReply *ButtonReplyData `json:"button_reply,omitempty"`
	ListReply   *ListReplyData   `json:"list_reply,omitempty"`
}

type ButtonReplyData struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ListReplyData struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type MessageContext struct {
	From string `json:"from"`
	ID   string `json:"id"`
}

type Status struct {
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	Timestamp   string       `json:"timestamp"`
	RecipientID string       `json:"recipient_id"`
	Conversation *ConversationInfo `json:"conversation,omitempty"`
	Pricing     *PricingInfo `json:"pricing,omitempty"`
}

type ConversationInfo struct {
	ID                  string           `json:"id"`
	Origin              *ConversationOrigin `json:"origin,omitempty"`
	ExpirationTimestamp string           `json:"expiration_timestamp,omitempty"`
}

type ConversationOrigin struct {
	Type string `json:"type"`
}

type PricingInfo struct {
	Billable     bool   `json:"billable"`
	PricingModel string `json:"pricing_model"`
	Category     string `json:"category"`
}

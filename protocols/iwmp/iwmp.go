package iwmp

// Attachment represents a mail attachment carried inside an IWMP message.
type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	DataB64     string `json:"data_b64,omitempty"`
}

// MailPayload is the end-to-end encrypted inner payload of a kind="mail" DTN bundle.
type MailPayload struct {
	MailID      string       `json:"mail_id"`
	ThreadID    string       `json:"thread_id,omitempty"`
	Subject     string       `json:"subject"`
	Body        string       `json:"body"`
	Attachments []Attachment `json:"attachments,omitempty"`
	SentAt      int64        `json:"sent_at"`
}

// MailRecord represents a stored mailbox item (inbox, outbox, drafts, sent).
type MailRecord struct {
	ID          string       `json:"id"`
	ThreadID    string       `json:"thread_id,omitempty"`
	IslandID    string       `json:"island_id,omitempty"`
	FromID      string       `json:"from_id"`
	ToID        string       `json:"to_id"`
	Subject     string       `json:"subject"`
	Body        string       `json:"body"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Folder      string       `json:"folder"` // "inbox", "outbox", "drafts", "sent"
	Status      string `json:"status"` // "draft", "queued", "delivered", "acked"
	Timestamp   int64  `json:"timestamp"`
}

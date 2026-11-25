package domain

import "time"

type AlertType string

const (
	AlertTypeNoReply   AlertType = "no_reply"
	AlertTypeComplaint AlertType = "complaint"
	AlertTypeRefusal   AlertType = "refusal"
	AlertTypeNegative  AlertType = "negative"
	AlertTypeUrgent    AlertType = "urgent"
)

// Alert — уведомление о проблеме в диалоге; nil в ResolvedAt означает, что оно не обработано.
type Alert struct {
	ID         int64      `json:"id" db:"id"`
	CompanyID  int64      `json:"company_id" db:"company_id"`
	ChatID     int64      `json:"chat_id" db:"chat_id"`
	Type       AlertType  `json:"type" db:"type"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at" db:"resolved_at"`
}

// NoReplyChat описывает просроченный диалог без ответа, для которого требуется создать no_reply-алерт.
type NoReplyChat struct {
	CompanyID int64 `db:"company_id"`
	ChatID    int64 `db:"chat_id"`
}

package domain

import "time"

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// ChatSummary — актуальная AI-выжимка, одна на ChatID.
// LastMessageID ссылается на внутренний Message.ID, до которого выполнен анализ.
type ChatSummary struct {
	ChatID        int64     `json:"chat_id" db:"chat_id"`
	Topic         string    `json:"topic" db:"topic"`
	Agreements    string    `json:"agreements" db:"agreements"`
	NextStep      string    `json:"next_step" db:"next_step"`
	OpenQuestions string    `json:"open_questions" db:"open_questions"`
	Risk          RiskLevel `json:"risk" db:"risk"`
	LastMessageID int64     `json:"last_message_id" db:"last_message_id"`
	GeneratedAt   time.Time `json:"generated_at" db:"generated_at"`
}

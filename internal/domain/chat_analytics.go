package domain

import "time"

type ChatDailyActivity struct {
	Day         string `json:"day"`
	MessagesIn  int    `json:"messages_in"`
	MessagesOut int    `json:"messages_out"`
}

// ChatAnalytics covers the complete stored history. Response times are seconds.
type ChatAnalytics struct {
	ChatID             int64               `json:"chat_id"`
	TotalMessages      int                 `json:"total_messages"`
	MessagesIn         int                 `json:"messages_in"`
	MessagesOut        int                 `json:"messages_out"`
	MediaMessages      int                 `json:"media_messages"`
	ResponseCount      int                 `json:"response_count"`
	AvgResponseSeconds *float64            `json:"avg_response_seconds"`
	MaxResponseSeconds *float64            `json:"max_response_seconds"`
	UnansweredMessages int                 `json:"unanswered_messages"`
	AwaitingReplySince *time.Time          `json:"awaiting_reply_since"`
	FirstMessageAt     *time.Time          `json:"first_message_at"`
	LastMessageAt      *time.Time          `json:"last_message_at"`
	DailyActivity      []ChatDailyActivity `json:"daily_activity"`
}

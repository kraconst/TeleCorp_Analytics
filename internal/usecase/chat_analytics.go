package usecase

import (
	"context"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

// Analytics reads messages through the same company-scoped access checks as history.
func (u *MessageUsecase) Analytics(ctx context.Context, companyID, chatID int64) (*domain.ChatAnalytics, error) {
	a := newChatAnalytics(chatID)
	var totalResponse float64
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		messages, err := u.ListByChat(ctx, companyID, chatID, pageSize, offset)
		if err != nil {
			return nil, err
		}
		for _, m := range messages {
			accumulateChatMessage(a, m, &totalResponse)
		}
		if len(messages) < pageSize {
			break
		}
	}
	return a, nil
}

func newChatAnalytics(chatID int64) *domain.ChatAnalytics {
	return &domain.ChatAnalytics{ChatID: chatID, DailyActivity: make([]domain.ChatDailyActivity, 0)}
}

// Messages must be ordered by sent_at, then ID. Consecutive incoming messages
// form one request; the first outgoing message answers it from its earliest time.
func accumulateChatMessage(a *domain.ChatAnalytics, m domain.Message, totalResponse *float64) {
	t := m.SentAt.UTC()
	if a.FirstMessageAt == nil {
		first := t
		a.FirstMessageAt = &first
	}
	a.LastMessageAt = &t
	a.TotalMessages++
	if m.MediaType != nil && *m.MediaType != "" {
		a.MediaMessages++
	}
	day := t.Format(time.DateOnly)
	if len(a.DailyActivity) == 0 || a.DailyActivity[len(a.DailyActivity)-1].Day != day {
		a.DailyActivity = append(a.DailyActivity, domain.ChatDailyActivity{Day: day})
	}
	daily := &a.DailyActivity[len(a.DailyActivity)-1]
	switch m.Direction {
	case domain.MessageDirectionIn:
		a.MessagesIn++
		daily.MessagesIn++
		a.UnansweredMessages++
		if a.AwaitingReplySince == nil {
			a.AwaitingReplySince = &t
		}
	case domain.MessageDirectionOut:
		a.MessagesOut++
		daily.MessagesOut++
		if a.AwaitingReplySince != nil {
			seconds := t.Sub(*a.AwaitingReplySince).Seconds()
			a.ResponseCount++
			*totalResponse += seconds
			avg := *totalResponse / float64(a.ResponseCount)
			a.AvgResponseSeconds = &avg
			if a.MaxResponseSeconds == nil || seconds > *a.MaxResponseSeconds {
				a.MaxResponseSeconds = &seconds
			}
			a.AwaitingReplySince = nil
			a.UnansweredMessages = 0
		}
	}
}

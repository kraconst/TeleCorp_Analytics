package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

type analyticsMessages struct {
	messages  []domain.Message
	err       error
	companyID int64
	calls     int
	failAt    int
}

func (s *analyticsMessages) Create(context.Context, domain.Message) (int64, error) { return 0, nil }
func (s *analyticsMessages) ListByChat(_ context.Context, companyID, _ int64, limit, offset int) ([]domain.Message, error) {
	s.companyID = companyID
	s.calls++
	if s.err != nil && (s.failAt == 0 || s.calls == s.failAt) {
		return nil, s.err
	}
	if offset >= len(s.messages) {
		return nil, nil
	}
	end := offset + limit
	if end > len(s.messages) {
		end = len(s.messages)
	}
	return s.messages[offset:end], nil
}

func TestChatAnalyticsLaterPageError(t *testing.T) {
	repo := &analyticsMessages{
		messages: make([]domain.Message, 200),
		err:      errors.New("second page failed"),
		failAt:   2,
	}
	a, err := NewMessageUsecase(repo, nil, nil).Analytics(context.Background(), 1, 1)
	if a != nil || !errors.Is(err, repo.err) || repo.calls != 2 {
		t.Fatalf("must discard partial analytics: result=%+v error=%v calls=%d", a, err, repo.calls)
	}
}

func TestChatAnalyticsMediaTypes(t *testing.T) {
	empty := domain.MediaType("")
	photo := domain.MediaTypePhoto
	repo := &analyticsMessages{messages: []domain.Message{
		{Direction: domain.MessageDirectionOut},
		{Direction: domain.MessageDirectionOut, MediaType: &empty},
		{Direction: domain.MessageDirectionOut, MediaType: &photo},
	}}
	a, err := NewMessageUsecase(repo, nil, nil).Analytics(context.Background(), 1, 1)
	if err != nil || a.TotalMessages != 3 || a.MediaMessages != 1 {
		t.Fatalf("media: %+v %v", a, err)
	}
}

func TestChatAnalyticsResponseBursts(t *testing.T) {
	base := time.Date(2026, 1, 1, 23, 58, 0, 0, time.UTC)
	media := domain.MediaTypePhoto
	repo := &analyticsMessages{messages: []domain.Message{
		{Direction: domain.MessageDirectionOut, SentAt: base},
		{Direction: domain.MessageDirectionIn, SentAt: base.Add(time.Minute)},
		{Direction: domain.MessageDirectionIn, SentAt: base.Add(2 * time.Minute), MediaType: &media},
		{Direction: domain.MessageDirectionOut, SentAt: base.Add(3 * time.Minute)},
		{Direction: domain.MessageDirectionOut, SentAt: base.Add(4 * time.Minute)},
		{Direction: domain.MessageDirectionIn, SentAt: base.Add(5 * time.Minute)},
		{Direction: domain.MessageDirectionOut, SentAt: base.Add(6 * time.Minute)},
		{Direction: domain.MessageDirectionIn, SentAt: base.Add(7 * time.Minute)},
	}}
	a, err := NewMessageUsecase(repo, nil, nil).Analytics(context.Background(), 42, 9)
	if err != nil {
		t.Fatal(err)
	}
	if a.TotalMessages != 8 || a.MessagesIn != 4 || a.MessagesOut != 4 || a.MediaMessages != 1 {
		t.Fatalf("counts: %+v", a)
	}
	if a.ResponseCount != 2 || a.AvgResponseSeconds == nil || *a.AvgResponseSeconds != 90 || *a.MaxResponseSeconds != 120 {
		t.Fatalf("responses: %+v", a)
	}
	if a.UnansweredMessages != 1 || !a.AwaitingReplySince.Equal(base.Add(7*time.Minute)) {
		t.Fatalf("pending: %+v", a)
	}
	if len(a.DailyActivity) != 2 || a.DailyActivity[0].MessagesIn != 1 || a.DailyActivity[1].MessagesIn != 3 {
		t.Fatalf("days: %+v", a.DailyActivity)
	}
	if repo.companyID != 42 || a.ChatID != 9 {
		t.Fatal("scope lost")
	}
}

func TestChatAnalyticsPagination(t *testing.T) {
	base := time.Now().UTC()
	repo := &analyticsMessages{}
	for i := 0; i < 200; i++ {
		repo.messages = append(repo.messages, domain.Message{Direction: domain.MessageDirectionIn, SentAt: base})
	}
	repo.messages = append(repo.messages, domain.Message{Direction: domain.MessageDirectionOut, SentAt: base.Add(time.Minute)})
	a, err := NewMessageUsecase(repo, nil, nil).Analytics(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if repo.calls != 2 || a.TotalMessages != 201 || a.ResponseCount != 1 || *a.AvgResponseSeconds != 60 || a.UnansweredMessages != 0 {
		t.Fatalf("pagination: %+v", a)
	}
}

func TestChatAnalyticsEmptyAndErrors(t *testing.T) {
	repo := &analyticsMessages{}
	u := NewMessageUsecase(repo, nil, nil)
	a, err := u.Analytics(context.Background(), 1, 1)
	if err != nil || a.DailyActivity == nil || a.AvgResponseSeconds != nil || a.FirstMessageAt != nil {
		t.Fatalf("empty: %+v %v", a, err)
	}
	if _, err := u.Analytics(context.Background(), 0, 1); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("validation: %v", err)
	}
	repo.err = errors.New("storage unavailable")
	if _, err := u.Analytics(context.Background(), 1, 1); !errors.Is(err, repo.err) {
		t.Fatalf("storage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := u.Analytics(ctx, 1, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestChatAnalyticsPendingAndInstantReply(t *testing.T) {
	base := time.Date(2026, 1, 2, 1, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	repo := &analyticsMessages{messages: []domain.Message{
		{Direction: domain.MessageDirectionIn, SentAt: base},
		{Direction: domain.MessageDirectionIn, SentAt: base.Add(time.Minute)},
	}}
	u := NewMessageUsecase(repo, nil, nil)
	a, err := u.Analytics(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.UnansweredMessages != 2 || a.ResponseCount != 0 || a.AvgResponseSeconds != nil || a.MaxResponseSeconds != nil || !a.AwaitingReplySince.Equal(base) {
		t.Fatalf("pending: %+v", a)
	}
	if a.DailyActivity[0].Day != "2026-01-01" {
		t.Fatal("day must use UTC")
	}
	repo.messages = []domain.Message{
		{Direction: domain.MessageDirectionIn, SentAt: base},
		{Direction: domain.MessageDirectionOut, SentAt: base},
	}
	a, err = u.Analytics(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.ResponseCount != 1 || a.AvgResponseSeconds == nil || *a.AvgResponseSeconds != 0 || a.AwaitingReplySince != nil {
		t.Fatalf("instant: %+v", a)
	}
}

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type stubNoReplyChecker struct {
	calls atomic.Int32
	err   error
	tick  chan struct{}
}

func (s *stubNoReplyChecker) CheckNoReplyAlerts(ctx context.Context) (int, error) {
	s.calls.Add(1)
	if s.tick != nil {
		select {
		case s.tick <- struct{}{}:
		default:
		}
	}
	return 0, s.err
}

func TestRunNoReplyAlertWorker_PeriodicAndGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checker := &stubNoReplyChecker{
		err:  errors.New("transient db error"),
		tick: make(chan struct{}, 8),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runNoReplyAlertWorker(ctx, checker, 10*time.Millisecond)
	}()

	// Wait for at least 2 ticks to confirm periodic execution even on error.
	for range 2 {
		select {
		case <-checker.tick:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for periodic no-reply check")
		}
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}

	if checker.calls.Load() < 2 {
		t.Fatalf("expected at least 2 checks, got %d", checker.calls.Load())
	}
}

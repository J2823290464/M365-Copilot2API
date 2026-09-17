package web

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAccountQueueTimeoutMapsToQueueError ensures acquireAccountSlot does not
// let the queue wait consume the whole request budget: the Acquire call gets a
// short independent deadline and its timeout maps to ErrAccountQueueTimeout.
func TestAccountQueueTimeoutMapsToQueueError(t *testing.T) {
	rs := defaultRuntimeSettings()
	rs.AccountQueueTimeoutSeconds = 1
	s := &Server{
		accountConcurrency: newAccountConcurrency(),
		settings:           &settingsStore{v: rs},
	}
	// Occupy every slot for account-a so the next acquire must queue.
	releases := make([]func(), 0, s.accountConcurrency.limit)
	for i := 0; i < s.accountConcurrency.limit; i++ {
		release, err := s.acquireAccountSlot(context.Background(), "account-a")
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, r := range releases {
			r()
		}
	}()

	start := time.Now()
	_, err := s.acquireAccountSlot(context.Background(), "account-a")
	if !errors.Is(err, ErrAccountQueueTimeout) {
		t.Fatalf("Acquire() error = %v, want ErrAccountQueueTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("queue wait took %v, want bounded by account queue timeout", elapsed)
	}
}

// TestAttemptContextBudgets verifies per-attempt budgets: 35s for the first
// attempt, 25s for the second, and an earlier parent deadline still wins.
func TestAttemptContextBudgets(t *testing.T) {
	first, cancel := attemptContext(context.Background(), 1, 0)
	defer cancel()
	if d, ok := first.Deadline(); !ok {
		t.Fatal("attempt 1 ctx has no deadline")
	} else if budget := time.Until(d); budget > attemptOneBudget+time.Second || budget < attemptOneBudget-time.Second {
		t.Fatalf("attempt 1 budget = %v, want %v", budget, attemptOneBudget)
	}

	second, cancel2 := attemptContext(context.Background(), 2, 0)
	defer cancel2()
	if d, ok := second.Deadline(); !ok {
		t.Fatal("attempt 2 ctx has no deadline")
	} else if budget := time.Until(d); budget > attemptTwoBudget+time.Second || budget < attemptTwoBudget-time.Second {
		t.Fatalf("attempt 2 budget = %v, want %v", budget, attemptTwoBudget)
	}

	parent, cancel3 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel3()
	child, cancel4 := attemptContext(parent, 1, 0)
	defer cancel4()
	if d, ok := child.Deadline(); !ok {
		t.Fatal("child ctx has no deadline")
	} else if budget := time.Until(d); budget > 3*time.Second+400*time.Millisecond {
		t.Fatalf("parent deadline did not win: budget = %v", budget)
	}
}

// TestAttemptContextDefaultMarksFirstAttempt ensures callers that do not tag
// an attempt number are treated as the first account attempt.
func TestAttemptContextDefaultMarksFirstAttempt(t *testing.T) {
	ctx, cancel := attemptContext(context.Background(), accountAttemptFrom(context.Background()), 0)
	defer cancel()
	if d, ok := ctx.Deadline(); !ok {
		t.Fatal("default attempt ctx has no deadline")
	} else if budget := time.Until(d); budget > attemptOneBudget+time.Second || budget < attemptOneBudget-time.Second {
		t.Fatalf("default budget = %v, want %v", budget, attemptOneBudget)
	}
}

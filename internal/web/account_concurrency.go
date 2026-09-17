package web

import (
	"context"
	"encoding/json"
	"errors"
	"m365-copilot2api/internal/applog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"m365-copilot2api/internal/chathub"
)

const defaultAccountConcurrency = 8

// ErrAccountQueueTimeout reports that an account's concurrency slot did not
// become available before the queue deadline. It maps to a 429 response with
// X-M365-Failure-Stage: account_queue.
var ErrAccountQueueTimeout = errors.New("account_queue_timeout: selected account is busy")

const (
	accountQueueTimeoutFallback = 5 * time.Second

	maxAccountAttempts = 2
	attemptOneBudget   = 35 * time.Second
	attemptTwoBudget   = 25 * time.Second
	minFailoverBudget  = 15 * time.Second
)

type accountAttemptKey struct{}

// withAccountAttempt tags the current account attempt number so shared
// chatWithAccount* helpers can cap each attempt with its own budget.
func withAccountAttempt(ctx context.Context, attempt int) context.Context {
	return context.WithValue(ctx, accountAttemptKey{}, attempt)
}

func accountAttemptFrom(ctx context.Context) int {
	if v, ok := ctx.Value(accountAttemptKey{}).(int); ok && v >= 1 {
		return v
	}
	return 1
}

// attemptContext derives a per-attempt budget: 35s for the first account,
// 25s for the second (capped by the configured account attempt timeout when
// smaller); the parent deadline (total request budget) still wins.
func attemptContext(ctx context.Context, attempt int, configured time.Duration) (context.Context, context.CancelFunc) {
	budget := attemptOneBudget
	if attempt == 2 {
		budget = attemptTwoBudget
	}
	if configured > 0 && configured < budget {
		budget = configured
	}
	return context.WithTimeout(ctx, budget)
}

// remainingBudget returns the time left on ctx. It is used to forbid
// failover when there is not enough budget for another account attempt.
func remainingBudget(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			return remaining
		}
		return 0
	}
	return time.Hour
}

type accountConcurrency struct {
	mu       sync.Mutex
	limit    int
	inflight map[string]int
	changed  chan struct{}
}

func newAccountConcurrency() *accountConcurrency {
	limit := defaultAccountConcurrency
	if raw := strings.TrimSpace(os.Getenv("M365_ACCOUNT_DEFAULT_CONCURRENCY")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	return &accountConcurrency{limit: limit, inflight: map[string]int{}, changed: make(chan struct{})}
}

func (c *accountConcurrency) Available(accountID string) bool {
	if c == nil || accountID == "" {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inflight[accountID] < c.limit
}

func (c *accountConcurrency) Acquire(ctx context.Context, accountID string) (func(), error) {
	if c == nil || accountID == "" {
		return func() {}, nil
	}
	for {
		c.mu.Lock()
		if c.inflight[accountID] < c.limit {
			c.inflight[accountID]++
			c.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					c.mu.Lock()
					if c.inflight[accountID] <= 1 {
						delete(c.inflight, accountID)
					} else {
						c.inflight[accountID]--
					}
					close(c.changed)
					c.changed = make(chan struct{})
					c.mu.Unlock()
				})
			}, nil
		}
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

func (c *accountConcurrency) Snapshot() map[string]any {
	if c == nil {
		return map[string]any{"limit": defaultAccountConcurrency, "inflight": map[string]int{}}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	inflight := make(map[string]int, len(c.inflight))
	for accountID, count := range c.inflight {
		inflight[accountID] = count
	}
	return map[string]any{"limit": c.limit, "inflight": inflight}
}

func (c *accountConcurrency) Inflight(accountID string) int {
	if c == nil || accountID == "" {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inflight[accountID]
}

func (s *Server) accountAvailable(accountID string) bool {
	if s.tokens != nil && !s.tokens.ScheduleEnabled(accountID) {
		return false
	}
	return s.accountPool.Available(accountID) && s.accountConcurrency.Available(accountID)
}

func (s *Server) activeSessionCounts() map[string]int {
	if s == nil || s.sessionResolver == nil {
		return map[string]int{}
	}
	return s.sessionResolver.ActiveSessionCounts()
}

func (s *Server) accountClient(accountID string) *chathub.Client {
	if acc, ok := s.tokens.Get(accountID); ok && acc.BoundProxy != "" {
		return s.clientForProxy(acc.BoundProxy)
	}
	return s.chat
}

func (s *Server) acquireAccountSlot(ctx context.Context, accountID string) (func(), error) {
	timeout := time.Duration(s.settings.get().AccountQueueTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = accountQueueTimeoutFallback
	}
	queueCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	release, err := s.accountConcurrency.Acquire(queueCtx, accountID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrAccountQueueTimeout
		}
		return nil, err
	}
	return release, nil
}

// chatWithAccountAttempt runs one account attempt with a dedicated per-attempt
// budget. attempt 1 gets 35s, attempt 2 gets 25s; the total request deadline
// still wins. Queueing uses its own short timeout so Acquire cannot consume
// the whole request budget.
func (s *Server) chatWithAccountAttempt(ctx context.Context, accountID string, account chathub.Account, request chathub.Request, attempt int) (chathub.Result, error) {
	configured := time.Duration(s.settings.get().AccountAttemptTimeoutSeconds) * time.Second
	if request.IdleTimeout <= 0 {
		request.IdleTimeout = time.Duration(s.settings.get().ChatIdleTimeoutSeconds) * time.Second
	}
	attemptCtx, cancel := attemptContext(ctx, attempt, configured)
	defer cancel()
	release, err := s.acquireAccountSlot(attemptCtx, accountID)
	if err != nil {
		return chathub.Result{}, err
	}
	defer release()
	if s.accountPool != nil {
		s.accountPool.MarkCall(accountID)
	}
	result, err := s.accountClient(accountID).Chat(attemptCtx, account, request)
	s.logChatHubUsage(request, result, err)
	s.recordAccountChatResult(accountID, result, err)
	return result, err
}

func (s *Server) chatWithAccount(ctx context.Context, accountID string, account chathub.Account, request chathub.Request) (chathub.Result, error) {
	return s.chatWithAccountAttempt(ctx, accountID, account, request, accountAttemptFrom(ctx))
}

func (s *Server) chatWithAccountEvents(ctx context.Context, accountID string, account chathub.Account, request chathub.Request, onEvent func(chathub.StreamEvent) error) (chathub.Result, error) {
	configured := time.Duration(s.settings.get().AccountAttemptTimeoutSeconds) * time.Second
	if request.IdleTimeout <= 0 {
		request.IdleTimeout = time.Duration(s.settings.get().ChatIdleTimeoutSeconds) * time.Second
	}
	attemptCtx, cancel := attemptContext(ctx, accountAttemptFrom(ctx), configured)
	defer cancel()
	release, err := s.acquireAccountSlot(attemptCtx, accountID)
	if err != nil {
		return chathub.Result{}, err
	}
	defer release()
	if s.accountPool != nil {
		s.accountPool.MarkCall(accountID)
	}
	result, err := s.accountClient(accountID).ChatWithEvents(attemptCtx, account, request, onEvent)
	s.logChatHubUsage(request, result, err)
	s.recordAccountChatResult(accountID, result, err)
	return result, err
}

func (s *Server) chatWithAccountReasoning(ctx context.Context, accountID string, account chathub.Account, request chathub.Request, onDelta, onReasoning func(string) error) (chathub.Result, error) {
	configured := time.Duration(s.settings.get().AccountAttemptTimeoutSeconds) * time.Second
	if request.IdleTimeout <= 0 {
		request.IdleTimeout = time.Duration(s.settings.get().ChatIdleTimeoutSeconds) * time.Second
	}
	attemptCtx, cancel := attemptContext(ctx, accountAttemptFrom(ctx), configured)
	defer cancel()
	release, err := s.acquireAccountSlot(attemptCtx, accountID)
	if err != nil {
		return chathub.Result{}, err
	}
	defer release()
	if s.accountPool != nil {
		s.accountPool.MarkCall(accountID)
	}
	result, err := s.accountClient(accountID).ChatWithReasoning(attemptCtx, account, request, onDelta, onReasoning)
	s.logChatHubUsage(request, result, err)
	s.recordAccountChatResult(accountID, result, err)
	return result, err
}

func (s *Server) logChatHubUsage(request chathub.Request, result chathub.Result, err error) {
	toolSchemaBytes := 0
	if len(request.Tools) > 0 {
		if data, marshalErr := json.Marshal(request.Tools); marshalErr == nil {
			toolSchemaBytes = len(data)
		}
	}
	tokenCount, _ := tokenEstimator("gpt-5.6-sol")
	promptTokens := int64(tokenCount(request.Text))
	completionTokens := int64(tokenCount(result.Text))
	if result.Reasoning != "" {
		completionTokens += int64(tokenCount(result.Reasoning))
	}
	if toolSchemaBytes > 0 {
		promptTokens += int64(tokenCount(string(mustJSON(request.Tools))))
	}
	applog.Info("web", "chat_usage", "chat_hub_call_id", result.RequestID, "prompt_tokens", promptTokens, "completion_tokens", completionTokens, "total_tokens", promptTokens+completionTokens, "tool_schema_bytes", toolSchemaBytes, "history_bytes", request.HistoryBytes, "status", usageStatus(err))
}

func usageStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

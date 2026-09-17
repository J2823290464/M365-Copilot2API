package web

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"m365-copilot2api/internal/chathub"
)

type requestIDKey struct{}

const (
	requestIDHeader           = "X-Request-ID"
	m365TimeoutHeader         = "X-M365-Timeout-Seconds"
	m365AttemptCountHeader    = "X-M365-Attempt-Count"
	m365AccountIDHeader       = "X-M365-Account-ID"
	requestIDMaxLength        = 128
	accountIdentifierMaxSize  = 64
	defaultChatTimeoutSeconds = 120
)

func requestIDFrom(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

func normalizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > requestIDMaxLength {
		return uuid.NewString()
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
		default:
			return uuid.NewString()
		}
	}
	return value
}

func maskAccountID(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}
	if len(accountID) <= accountIdentifierMaxSize {
		return "masked-account"
	}
	return fmt.Sprintf("masked-account-%x", sha256.Sum256([]byte(accountID)))
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := normalizeRequestID(r.Header.Get(requestIDHeader))
		w.Header().Set(requestIDHeader, id)
		// Default request metadata is present on every response, including
		// early validation errors; chat handlers refine the account ID and
		// attempt count once an account is selected.
		setM365RequestHeaders(w, defaultChatTimeoutSeconds, 1, "")
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		ctx = chathub.WithTrace(ctx, &chathub.RequestTrace{RequestID: id})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func setM365RequestHeaders(w http.ResponseWriter, timeoutSeconds, attemptCount int, accountID string) {
	if w == nil {
		return
	}
	w.Header().Set(m365TimeoutHeader, fmt.Sprintf("%d", timeoutSeconds))
	w.Header().Set(m365AttemptCountHeader, fmt.Sprintf("%d", attemptCount))
	if masked := maskAccountID(accountID); masked != "" {
		w.Header().Set(m365AccountIDHeader, masked)
	} else {
		w.Header().Del(m365AccountIDHeader)
	}
}

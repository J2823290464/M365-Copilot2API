package web

import (
	"crypto/sha256"
	"encoding/hex"
	"m365-copilot2api/internal/chathub"
	"strings"
	"sync"
	"time"
)

type cachedConversation struct {
	ConversationID string
	SessionID      string
	Tone           string
	TurnCount      int
	MessageCount   int
	CreatedAt      time.Time
	LastUsedAt     time.Time
	SystemPrompt   string
	SessionFinger  string
}

type conversationCache struct {
	mu      sync.Mutex
	entries map[string]*cachedConversation
	maxAge  time.Duration
}

func newConversationCache() *conversationCache {
	return &conversationCache{entries: make(map[string]*cachedConversation), maxAge: 2 * time.Hour}
}

func (c *conversationCache) key(accountID, model, sessionFinger string) string {
	return accountID + "|" + model + "|" + sessionFinger
}

func (c *conversationCache) Lookup(accountID, model, sessionFinger string) *cachedConversation {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(accountID, model, sessionFinger)
	entry := c.entries[key]
	if entry == nil {
		return nil
	}
	if time.Since(entry.LastUsedAt) > c.maxAge {
		delete(c.entries, key)
		return nil
	}
	return entry
}

func (c *conversationCache) Store(accountID, model, sessionFinger string, conv *cachedConversation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conv.LastUsedAt = time.Now()
	c.entries[c.key(accountID, model, sessionFinger)] = conv
}

func (c *conversationCache) Invalidate(accountID, model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := accountID + "|" + model + "|"
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
}

func (c *conversationCache) GC() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, v := range c.entries {
		if now.Sub(v.LastUsedAt) > c.maxAge {
			delete(c.entries, k)
		}
	}
}

func (c *conversationCache) Stats() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]any{"cached_conversations": len(c.entries)}
}

func systemPromptHash(messages []oaiMsg) string {
	for _, m := range messages {
		if m.Role == "system" || m.Role == "developer" {
			h := sha256.Sum256([]byte(contentToString(m.Content)))
			return hex.EncodeToString(h[:])
		}
	}
	return ""
}

func sessionFingerprint(messages []oaiMsg) string {
	h := sha256.New()
	for _, m := range messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "system" || role == "developer" {
			continue
		}
		if role == "user" {
			_, _ = h.Write([]byte(contentToString(m.Content)))
			break
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func extractLastUserMessage(messages []oaiMsg) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return contentToString(messages[i].Content)
		}
	}
	return ""
}

func (s *Server) storeConvCache(accID, model string, res chathub.Result, tone string, messages []oaiMsg, reused bool) {
	if res.ConversationID == "" {
		return
	}
	finger := sessionFingerprint(messages)
	cached := s.convCache.Lookup(accID, model, finger)
	entry := &cachedConversation{
		ConversationID: res.ConversationID,
		SessionID:      res.SessionID,
		Tone:           tone,
		MessageCount:   len(messages),
		SystemPrompt:   systemPromptHash(messages),
		SessionFinger:  finger,
	}
	if cached != nil && cached.ConversationID == res.ConversationID {
		entry.TurnCount = cached.TurnCount + 1
	} else {
		entry.TurnCount = 1
	}
	s.convCache.Store(accID, model, finger, entry)
}

func (s *Server) invalidateConvCache(accID, model string) {
	s.convCache.Invalidate(accID, model)
}

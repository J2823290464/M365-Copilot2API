package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

func serializedMessagesBytes(messages []oaiMsg) int {
	data, err := json.Marshal(messages)
	if err != nil {
		return 0
	}
	return len(data)
}

func localTitleRequest(body *oaiReq, prompt string) bool {
	if body == nil || body.Stream || len(body.Tools) != 0 || len(body.Messages) != 2 {
		return false
	}
	lower := strings.ToLower(prompt)
	if !strings.Contains(lower, "title") || !strings.Contains(lower, "json") {
		return false
	}
	return strings.Contains(lower, "conversation") ||
		strings.Contains(lower, "concise") ||
		strings.Contains(lower, "summarize")
}

func localTitle(messages []oaiMsg) string {
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		text := strings.TrimSpace(contentToString(message.Content))
		if text == "" {
			continue
		}
		for _, token := range strings.Fields(text) {
			token = strings.Trim(token, "`'\".,:;()[]{}")
			if strings.HasSuffix(strings.ToLower(token), ".md") {
				return token
			}
		}
		return conversationTitle([]oaiMsg{{Role: "user", Content: text}})
	}
	return "Untitled conversation"
}

func writeLocalTitleResponse(w http.ResponseWriter, model string, messages []oaiMsg) {
	content := mustJSON(map[string]string{"title": localTitle(messages)})
	jsonOut(w, map[string]any{
		"id":      "chatcmpl-local-title-" + uuid.NewString(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	})
}

package web

import (
	"context"
	"fmt"
	"log"

	"strings"

	"m365-copilot2api/internal/chathub"
)

type atomKind string

const (
	kindSystem atomKind = "SYSTEM"
	kindUser   atomKind = "USER"
	kindTool   atomKind = "ATOM_TOOL"
	kindAssist atomKind = "ASSIST"
	kindAnchor atomKind = "ANCHOR"
)

type contextAtom struct {
	Kind   atomKind
	Msgs   []oaiMsg
	Tokens int
	Start  int
	End    int
}

func estimateBudgetTokens(text string) int {
	return heuristicTokenCount(text)
}

func estimateMessageTokens(m oaiMsg, counter func(string) int) int {
	if counter == nil {
		counter = estimateBudgetTokens
	}
	tokens := messageProtocolTokens
	tokens += counter(m.Role)
	tokens += counter(m.Name)
	tokens += counter(m.ToolCallID)
	tokens += serializedTokenCount(m.Content, counter)
	for _, call := range m.ToolCalls {
		tokens += serializedTokenCount(call, counter)
	}
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

func buildAtomsFast(messages []oaiMsg) []contextAtom {
	return buildAtomsWithCounter(messages, heuristicTokenCount)
}

func buildAtoms(messages []oaiMsg) []contextAtom {
	counter, _ := tokenEstimator("gpt-4")
	if counter == nil {
		counter = heuristicTokenCount
	}
	return buildAtomsWithCounter(messages, counter)
}

func buildAtomsWithCounter(messages []oaiMsg, counter func(string) int) []contextAtom {
	if len(messages) == 0 {
		return nil
	}
	var atoms []contextAtom
	i := 0
	for i < len(messages) {
		m := messages[i]
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "system" || role == "developer" {
			start := i
			var msgs []oaiMsg
			total := 0
			for i < len(messages) {
				r := strings.ToLower(strings.TrimSpace(messages[i].Role))
				if r != "system" && r != "developer" {
					break
				}
				msgs = append(msgs, messages[i])
				total += estimateMessageTokens(messages[i], counter)
				i++
			}
			atoms = append(atoms, contextAtom{Kind: kindSystem, Msgs: msgs, Tokens: total, Start: start, End: i})
			continue
		}
		if role == "assistant" && len(m.ToolCalls) > 0 {
			start := i
			var msgs []oaiMsg
			total := 0
			msgs = append(msgs, m)
			total += estimateMessageTokens(m, counter)
			i++
			for i < len(messages) && strings.ToLower(strings.TrimSpace(messages[i].Role)) == "tool" {
				msgs = append(msgs, messages[i])
				total += estimateMessageTokens(messages[i], counter)
				i++
			}
			atoms = append(atoms, contextAtom{Kind: kindTool, Msgs: msgs, Tokens: total, Start: start, End: i})
			continue
		}
		if role == "tool" {
			start := i
			var msgs []oaiMsg
			total := 0
			for i < len(messages) && strings.ToLower(strings.TrimSpace(messages[i].Role)) == "tool" {
				msgs = append(msgs, messages[i])
				total += estimateMessageTokens(messages[i], counter)
				i++
			}
			atoms = append(atoms, contextAtom{Kind: kindTool, Msgs: msgs, Tokens: total, Start: start, End: i})
			continue
		}
		if role == "user" {
			atoms = append(atoms, contextAtom{Kind: kindUser, Msgs: []oaiMsg{m}, Tokens: estimateMessageTokens(m, counter), Start: i, End: i + 1})
			i++
			continue
		}
		if role == "assistant" {
			atoms = append(atoms, contextAtom{Kind: kindAssist, Msgs: []oaiMsg{m}, Tokens: estimateMessageTokens(m, counter), Start: i, End: i + 1})
			i++
			continue
		}
		atoms = append(atoms, contextAtom{Kind: kindUser, Msgs: []oaiMsg{m}, Tokens: estimateMessageTokens(m, counter), Start: i, End: i + 1})
		i++
	}
	for idx, a := range atoms {
		if a.Kind == kindUser {
			atoms[idx].Kind = kindAnchor
			break
		}
	}
	return atoms
}

func flattenAtoms(atoms []contextAtom, attachments []chathub.Attachment) (string, []chathub.Attachment) {
	var msgs []oaiMsg
	for _, a := range atoms {
		msgs = append(msgs, a.Msgs...)
	}
	return flattenPromptMessages(msgs, attachments)
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for limit > 0 && (value[limit]&0xc0) == 0x80 {
		limit--
	}
	return value[:limit]
}

const contextSummaryPrompt = `Summarize the earlier conversation for another assistant. Preserve decisions, requirements, constraints, file paths, error messages, tool results, and unresolved tasks. Remove greetings, repetition, and narration. Be concise and factual. Do not answer the user's latest request. Return only the summary.`

func splitContextForCompression(messages []oaiMsg) (system, oldHistory, currentTurn []oaiMsg, ok bool) {
	lastUser := -1
	for i, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			lastUser = i
		}
	}
	if lastUser <= 0 {
		return nil, nil, messages, false
	}
	for i, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role == "system" || role == "developer" {
			system = append(system, message)
			continue
		}
		if i < lastUser {
			oldHistory = append(oldHistory, message)
		} else {
			currentTurn = append(currentTurn, message)
		}
	}
	return system, oldHistory, currentTurn, len(oldHistory) > 0 && len(currentTurn) > 0
}

func (s *Server) autoCompressContext(ctx context.Context, accountID string, account chathub.Account, messages []oaiMsg, tone, licenseType, scenario string) ([]oaiMsg, bool) {
	system, oldHistory, currentTurn, ok := splitContextForCompression(messages)
	if !ok {
		return messages, false
	}
	oldPrompt, _ := flattenPromptMessages(oldHistory, nil)
	oldPrompt = strings.TrimSpace(oldPrompt)
	if oldPrompt == "" {
		return messages, false
	}
	if len(oldPrompt) > 24000 {
		oldPrompt = truncateUTF8(oldPrompt, 24000)
	}
	result, err := s.chatWithAccount(ctx, accountID, account, chathub.Request{
		Text:        contextSummaryPrompt + "\n\nEARLIER CONVERSATION:\n" + oldPrompt,
		Tone:        tone,
		LicenseType: licenseType,
		Scenario:    scenario,
	})
	if result.ConversationID != "" {
		s.dropTransientConversation(result.ConversationID)
	}
	if err != nil || strings.TrimSpace(result.Text) == "" {
		log.Printf("[context-compress] failed account=%s err=%v", accountID, err)
		return messages, false
	}
	summary := strings.TrimSpace(result.Text)
	compressed := make([]oaiMsg, 0, len(system)+len(currentTurn)+1)
	compressed = append(compressed, system...)
	compressed = append(compressed, oaiMsg{Role: "system", Content: "[conversation summary]\n" + summary})
	compressed = append(compressed, currentTurn...)
	log.Printf("[context-compress] account=%s old_messages=%d summary_chars=%d", accountID, len(oldHistory), len(summary))
	return compressed, true
}
func flattenPromptMessagesWithBudget(messages []oaiMsg, attachments []chathub.Attachment, budget int) (string, []chathub.Attachment, bool, error) {
	truncatedMsgs, truncated, err := slidingWindow(messages, budget)
	if err != nil {
		return "", attachments, false, err
	}
	prompt, atts := flattenPromptMessages(truncatedMsgs, attachments)
	return prompt, atts, truncated, nil
}

// budget for slidingWindow: B = ContextWindow - MaxOutput - 512
func slidingWindow(messages []oaiMsg, budget int) ([]oaiMsg, bool, error) {
	if budget <= 0 {
		budget = 1024
	}
	atoms := buildAtomsFast(messages)
	if len(atoms) == 0 {
		return messages, false, nil
	}
	total := 0
	for _, a := range atoms {
		total += a.Tokens
	}
	total += requestProtocolTokens + replyPrimingTokens
	if total <= budget {
		return messages, false, nil
	}
	var p0Indices []int
	anchorIdx := -1
	for idx, a := range atoms {
		if a.Kind == kindSystem {
			p0Indices = append(p0Indices, idx)
		}
		if a.Kind == kindAnchor && anchorIdx == -1 {
			anchorIdx = idx
		}
	}
	var p1Indices []int
	for idx := len(atoms) - 1; idx >= 0; idx-- {
		if atoms[idx].Kind == kindTool {
			p1Indices = append([]int{idx}, p1Indices...)
		} else {
			if len(p1Indices) > 0 {
				break
			}
			break
		}
	}
	sumP0P1 := requestProtocolTokens + replyPrimingTokens
	for _, idx := range p0Indices {
		sumP0P1 += atoms[idx].Tokens
	}
	for _, idx := range p1Indices {
		sumP0P1 += atoms[idx].Tokens
	}
	if anchorIdx != -1 {
		sumP0P1 += atoms[anchorIdx].Tokens
	}
	if sumP0P1 > budget {
		return nil, false, fmt.Errorf("context_length_exceeded: pinned context (system+current task+anchor) %d tokens exceed budget %d; reduce tool results or start a new session", sumP0P1, budget)
	}
	remaining := budget - sumP0P1
	selected := make(map[int]bool)
	for _, idx := range p0Indices {
		selected[idx] = true
	}
	for _, idx := range p1Indices {
		selected[idx] = true
	}
	if anchorIdx != -1 {
		selected[anchorIdx] = true
	}
	for idx := len(atoms) - 1; idx >= 0; idx-- {
		if selected[idx] {
			continue
		}
		tok := atoms[idx].Tokens
		if tok <= remaining {
			selected[idx] = true
			remaining -= tok
		}
	}
	var out []oaiMsg
	for idx, a := range atoms {
		if selected[idx] {
			out = append(out, a.Msgs...)
		}
	}
	truncated := len(selected) < len(atoms)
	if len(out) == 0 && len(atoms) > 0 {
		last := atoms[len(atoms)-1]
		out = append(out, last.Msgs...)
		truncated = true
	}
	return out, truncated, nil
}

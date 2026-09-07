package web

import (
	"encoding/json"
	"fmt"
	"strings"
)

// maxRouterContextChars caps how much of the flattened conversation is embedded in a
// tool-routing prompt. 50 tool schemas plus a long history pushed routing prompts
// past 150k chars (~37k tokens) and tripped the upstream 32768-token budget.
// Routing only needs the current request and recent turns, never the full history.
const maxRouterContextChars = 24000

// compactToolDefs replaces the full JSON schema dump with one line per tool.
// validateDetectedToolCalls re-validates every parsed call against the full schema.
func compactToolDefs(tools []map[string]any) string {
	var b strings.Builder
	for _, t := range tools {
		fn, _ := t["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}
		desc, _ := fn["description"].(string)
		if len(desc) > 160 {
			desc = desc[:160] + "..."
		}
		var required []string
		if params, ok := fn["parameters"].(map[string]any); ok {
			if req, ok := params["required"].([]any); ok {
				for _, r := range req {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
			}
		}
		if len(required) > 0 {
			fmt.Fprintf(&b, "- %s(required: %s): %s\n", name, strings.Join(required, ","), desc)
		} else {
			fmt.Fprintf(&b, "- %s: %s\n", name, desc)
		}
	}
	return strings.TrimSpace(b.String())
}

// trimRouterContext keeps the head (system/dev framing) and the tail (current
// request), dropping the middle of long histories so routing stays within budget.
func trimRouterContext(prompt string) string {
	if len(prompt) <= maxRouterContextChars {
		return prompt
	}
	head := maxRouterContextChars / 4
	tail := maxRouterContextChars - head
	return prompt[:head] + "\n...[older history omitted]...\n" + prompt[len(prompt)-tail:]
}
func modelToolRouterPrompt(prompt string, tools []map[string]any, choice any) string {
	defs := compactToolDefs(tools)
	mode := normalizedToolChoiceMode(choice)
	rules := `- If a tool is needed, respond with: CALL_TOOL: tool_name({"arg1":"value1"})
- If no tool is needed, respond with: NO_TOOL_NEEDED
- Only use tools from the available list above
- Validate all arguments against the tool's schema
- Batch independent read-only operations in one decision: emit multiple calls for file listing, file search, code search, and reading unrelated files when the declared tools support them
- Prefer one broad search or a single shell inspection command over a chain of list-then-search-then-read calls when an equivalent declared tool is available
- Keep dependent operations and all mutations/execution in order; never parallelize a write, edit, delete, or command that depends on another result
- Do not invent tools that are not in the list`
	// Multi-turn: completed tool evidence (tool[...], tool_calls:) was already
	// acted upon, so re-invoking those tools would duplicate work.
	if strings.Contains(prompt, "tool_calls:") || strings.Contains(prompt, "tool[call_") {
		rules += `
- Completed evidence must not be repeated: tool_calls/tool[call_x] rows are prior results already delivered to the user, never re-invoke them
- Only start a new tool call when fresh unfinished work remains on the current request`
	}
	return fmt.Sprintf(`You are a tool selection assistant. Based on the user request, decide which tool to call next.

Available tools: %s

MODE: %s

Rules:
%s

User request and evidence:
%s`, defs, mode, rules, trimRouterContext(prompt))
}

func parseModelToolDecision(text string, tools []map[string]any, choice any) ([]detectedToolCall, bool) {
	text = strings.TrimSpace(text)
	// Try the new natural language format first: CALL_TOOL: name({...})
	if strings.HasPrefix(text, "CALL_TOOL:") || strings.HasPrefix(text, "call_tool:") {
		parts := strings.SplitN(text, ":", 2)
		if len(parts) == 2 {
			rest := strings.TrimSpace(parts[1])
			start := strings.Index(rest, "(")
			end := strings.LastIndex(rest, ")")
			if start > 0 && end > start {
				name := strings.TrimSpace(rest[:start])
				argsStr := rest[start+1 : end]
				var args map[string]any
				if json.Unmarshal([]byte(argsStr), &args) == nil && toolChoiceAllows(choice, name) {
					fn := toolFunction(name, tools)
					if fn != nil && schemaValid(args, fn) == nil {
						b, _ := json.Marshal(args)
						return []detectedToolCall{{ID: callID(name, string(b), 0), Type: toolType(name, tools), Name: name, Arguments: b}}, true
					}
				}
			}
		}
	}
	if strings.Contains(text, "NO_TOOL_NEEDED") || strings.Contains(text, "no_tool_needed") {
		return nil, true
	}
	// Fallback: try the old JSON format
	if i := strings.Index(text, "```"); i >= 0 {
		text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(text[i+3:], "```"), "json"))
	}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal([]byte(text[start:end+1]), &probe) != nil {
		return nil, false
	}
	if _, ok := probe["calls"]; !ok {
		return nil, false
	}
	var envelope struct {
		Calls []struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"calls"`
	}
	if json.Unmarshal([]byte(text[start:end+1]), &envelope) != nil {
		return nil, false
	}
	out := make([]detectedToolCall, 0, len(envelope.Calls))
	for i, c := range envelope.Calls {
		fn := toolFunction(c.Name, tools)
		if fn == nil || c.Arguments == nil || !toolChoiceAllows(choice, c.Name) || schemaValid(c.Arguments, fn) != nil {
			continue
		}
		b, _ := json.Marshal(c.Arguments)
		out = append(out, detectedToolCall{ID: callID(c.Name, string(b), i), Type: toolType(c.Name, tools), Name: c.Name, Arguments: b})
	}
	return out, true
}

func localToolIntent(prompt string) bool {
	low := strings.ToLower(prompt)
	keywords := []string{"修订", "审订", "核改", "批改", "更正", "订正", "微调", "局部调整", "结构性调整", "推翻重来", "版本迭代", "拟订", "增删", "措辞修改", "行文规范", "修改建议", "修订记录", "变更申请", "最终定稿", "版本回退",
	                    "润色", "打磨", "炼字", "降重", "扩写", "精炼", "重组", "增删", "调整", "优化", "审稿", "校对", "换角度", "情绪强化", "批注", "全局修改", "软性修改",
	                    "调色", "统一", "构图调整", "细化", "简化", "微交互优化", "动效参数调整", "间距", "视觉降噪", "迭代", "优化视觉表现", "输出切图", "还原走查", "微动一下", "细节收尾", "整体感把控",
	                     "重构", "优化", "修复", "补丁", "更新", "热修复", "回滚", "覆盖", "迁移", "Diff", "Commit", "Merge", "冲突解决",
	                     "优化一下", "细化", "复盘后调整", "换一种思路", "重新梳理", "微调", "调整", "补齐", "核对", "优化", "重组", "拆分", "精简", "重塑", "推翻", "重构", "重新定义", "增强", "减弱", "明确",
	                     "重写", "改写", "拼写纠错", "格式规范化", "时间调换", "顺序重排", "删减冗余", "替换配图", "音量调整", "裁剪拼接"}
	for _, keyword := range keywords {
		if strings.Contains(low, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func hasClientWorkspaceTool(tools []map[string]any) bool {
	for _, tool := range tools {
		if isExplicitClientWorkspaceTool(tool) {
			return true
		}
	}
	return false
}

func isExplicitClientWorkspaceTool(tool map[string]any) bool {
	for _, key := range []string{"client_side", "clientSide", "local", "workspace", "requires_client", "requiresClient"} {
		if value, ok := tool[key].(bool); ok && value {
			return true
		}
	}
	for _, key := range []string{"execution_target", "executionTarget", "tool_scope", "toolScope", "target"} {
		if value, ok := tool[key].(string); ok && isClientWorkspaceTarget(value) {
			return true
		}
	}
	for _, key := range []string{"metadata", "annotations"} {
		if value, ok := tool[key].(map[string]any); ok && isExplicitClientWorkspaceTool(value) {
			return true
		}
	}
	toolType, _ := tool["type"].(string)
	if isClientWorkspaceType(toolType) {
		return true
	}
	fn, _ := tool["function"].(map[string]any)
	if fn != nil {
		if isExplicitClientWorkspaceTool(fn) {
			return true
		}
		name, _ := fn["name"].(string)
		if isClientWorkspaceName(name) {
			return true
		}
	}
	return false
}

func isClientWorkspaceTarget(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "client", "client_side", "local", "workspace", "local_workspace":
		return true
	default:
		return false
	}
}

func isClientWorkspaceType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "client_tool", "local_tool", "workspace_tool", "client_function":
		return true
	default:
		return false
	}
}

func isClientWorkspaceName(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "exec", "execute", "shell", "terminal", "powershell", "bash", "read_file", "write_file", "edit_file", "apply_patch", "list_directory", "list_files", "search_files", "search_code", "run_test", "run_tests":
		return true
	default:
		return false
	}
}

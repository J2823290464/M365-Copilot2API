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
	keywords := []string{"源码", "源代码", "项目", "代码", "文件", "目录", "路径", "读取", "查看", "检查", "修改", "编辑", "测试", "编译", "source code", "project", "code", "file", "directory", "path", "read", "inspect", "modify", "edit", "test", "compile", "pom.xml", "package.json", "go.mod", "c:\\", "d:\\", "/workspace/", "/src/"}
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

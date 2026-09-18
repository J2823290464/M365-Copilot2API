package web

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"m365-copilot2api/internal/chathub"
)

type AutoToolRule struct {
	Patterns   []string
	MatchRegex bool
	ToolNames  []string
}

type clientToolKind string

const (
	clientToolKindCustomExec clientToolKind = "custom_exec"
	clientToolKindRead       clientToolKind = "read"
	clientToolKindWrite      clientToolKind = "write"
	clientToolKindSearch     clientToolKind = "search"
	clientToolKindShell      clientToolKind = "shell"
)

type clientToolDefinition struct {
	Name        string
	Type        string
	Description string
	Parameters  map[string]any
}

var defaultAutoToolRules = []AutoToolRule{
	{Patterns: []string{"read file", "view file", "open file", "查看文件", "读取文件", "打开文件"}, ToolNames: []string{"read_file"}},
	{Patterns: []string{"create file", "write file", "save file", "创建文件", "写入文件", "新建"}, ToolNames: []string{"write_file"}},
	{Patterns: []string{"search", "find", "grep", "搜索", "查找", "查询"}, ToolNames: []string{"search_files"}},
	{Patterns: []string{"run", "execute", "python", "npm", "docker", "运行", "执行"}, ToolNames: []string{"shell_command"}},
	{Patterns: []string{"edit", "modify", "replace", "patch", "编辑", "修改", "替换", "更新"}, ToolNames: []string{"read_file", "write_file"}},
}

var clientToolNamesByKind = map[clientToolKind][]string{
	clientToolKindCustomExec: {"exec"},
	clientToolKindRead:       {"read_file", "read", "Read", "view_file", "open_file"},
	clientToolKindWrite:      {"write_file", "write", "Write", "create_file", "save_file", "Edit", "apply_patch"},
	clientToolKindSearch:     {"search_files", "search_file", "find_files", "search", "Grep", "Glob"},
	clientToolKindShell:      {"shell_command", "powershell", "bash", "Bash", "sh", "shell", "terminal", "cmd"},
}

var autoToolSemanticNames = map[string]clientToolKind{
	"read_file":     clientToolKindRead,
	"write_file":    clientToolKindWrite,
	"search_files":  clientToolKindSearch,
	"shell_command": clientToolKindShell,
}

func autoSelectClientTools(prompt string, clientTools []chathub.Tool, rules []AutoToolRule) []chathub.Tool {
	definitions := parseClientToolDefinitions(clientTools)
	clientMode := clientToolMode(definitions)
	declaredNames := clientToolDefinitionNames(definitions)
	intentKinds := matchedAutoToolKinds(prompt, rules)
	if len(definitions) == 0 {
		log.Printf("[auto-tools] enabled=true action=client_selection client_mode=%s declared_tools=%v intents=%v selected_tools=%v reason=no_declared_tools", clientMode, declaredNames, intentKinds, []string{})
		return clientTools
	}
	if len(intentKinds) == 0 {
		log.Printf("[auto-tools] enabled=true action=client_selection client_mode=%s declared_tools=%v intents=%v selected_tools=%v reason=no_intent_match", clientMode, declaredNames, intentKinds, []string{})
		return clientTools
	}

	customExec := findClientToolDefinition(definitions, clientToolKindCustomExec)
	shell := findClientToolDefinition(definitions, clientToolKindShell)
	selected := make(map[string]bool, len(intentKinds))
	selectedNames := make([]string, 0, len(intentKinds))
	for _, kind := range intentKinds {
		definition := findClientToolDefinition(definitions, kind)
		if definition == nil && customExec != nil {
			definition = customExec
		}
		if definition == nil && shell != nil {
			definition = shell
		}
		if definition != nil {
			if !containsString(selectedNames, definition.Name) {
				selectedNames = append(selectedNames, definition.Name)
			}
			selected[definition.Name] = true
		}
	}
	if len(selected) == 0 {
		log.Printf("[auto-tools] enabled=true action=client_selection client_mode=%s declared_tools=%v intents=%v selected_tools=%v reason=no_matching_client_tool", clientMode, declaredNames, intentKinds, []string{})
		return clientTools
	}

	result := make([]chathub.Tool, 0, len(selected))
	for _, tool := range clientTools {
		if definition := parseClientToolDefinition(tool); definition != nil && selected[definition.Name] {
			result = append(result, tool)
		}
	}
	log.Printf("[auto-tools] enabled=true action=client_selection client_mode=%s declared_tools=%v intents=%v selected_tools=%v selected_count=%d/%d reason=matched", clientMode, declaredNames, intentKinds, selectedNames, len(result), len(clientTools))
	return result
}

func parseClientToolDefinitions(tools []chathub.Tool) []clientToolDefinition {
	definitions := make([]clientToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if definition := parseClientToolDefinition(tool); definition != nil {
			definitions = append(definitions, *definition)
		}
	}
	return definitions
}

func parseClientToolDefinition(tool chathub.Tool) *clientToolDefinition {
	var function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	}
	if json.Unmarshal(tool.Function, &function) != nil || strings.TrimSpace(function.Name) == "" {
		return nil
	}
	return &clientToolDefinition{
		Name:        strings.TrimSpace(function.Name),
		Type:        strings.TrimSpace(tool.Type),
		Description: function.Description,
		Parameters:  function.Parameters,
	}
}

func matchedAutoToolKinds(prompt string, rules []AutoToolRule) []clientToolKind {
	if len(rules) == 0 {
		rules = defaultAutoToolRules
	}
	lowerPrompt := strings.ToLower(prompt)
	kinds := make([]clientToolKind, 0)
	seen := make(map[clientToolKind]bool)
	for _, rule := range rules {
		for _, pattern := range rule.Patterns {
			matched := false
			if rule.MatchRegex {
				expression, err := regexp.Compile("(?i)" + pattern)
				matched = err == nil && expression.MatchString(prompt)
			} else {
				matched = strings.Contains(lowerPrompt, strings.ToLower(pattern))
			}
			if !matched {
				continue
			}
			for _, name := range rule.ToolNames {
				if kind, ok := autoToolSemanticNames[name]; ok && !seen[kind] {
					seen[kind] = true
					kinds = append(kinds, kind)
				}
			}
			break
		}
	}
	return kinds
}

func findClientToolDefinition(definitions []clientToolDefinition, kind clientToolKind) *clientToolDefinition {
	for _, preferredName := range clientToolNamesByKind[kind] {
		for index := range definitions {
			if strings.EqualFold(definitions[index].Name, preferredName) {
				return &definitions[index]
			}
		}
	}
	return nil
}

func clientToolMode(definitions []clientToolDefinition) string {
	if findClientToolDefinition(definitions, clientToolKindCustomExec) != nil {
		return "codex_custom_exec"
	}
	if findClientToolDefinition(definitions, clientToolKindShell) != nil {
		return "command_tool"
	}
	for _, kind := range []clientToolKind{clientToolKindRead, clientToolKindWrite, clientToolKindSearch} {
		if findClientToolDefinition(definitions, kind) != nil {
			return "named_tool"
		}
	}
	return "unknown"
}

func clientToolDefinitionNames(definitions []clientToolDefinition) []string {
	names := make([]string, len(definitions))
	for index := range definitions {
		names[index] = definitions[index].Name
	}
	return names
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

// routeClientTools applies the pass-through switch semantics:
//   - passThrough=true:  forward client-declared tools unchanged (no filter,
//     no fallback injection).
//   - passThrough=false: filter to intent-matched client tools, then inject
//     compatible base tools for intents the client did not cover.
//
// mcp__ tools are never injected by fallback logic; they stay on the gateway.
func routeClientTools(passThrough bool, prompt string, tools []chathub.Tool) []chathub.Tool {
	if passThrough {
		return tools
	}
	selected := autoSelectClientTools(prompt, tools, nil)
	declared := clientToolDefinitionNames(parseClientToolDefinitions(selected))
	kind := detectClientKind(declared)
	intentKinds := matchedAutoToolKinds(prompt, nil)
	fallback := injectFallbackTools(kind, declared, intentKinds)
	if len(fallback) == 0 {
		return selected
	}
	return append(selected, fallback...)
}

// clientKind identifies the calling agent so fallback injection can use tool
// names that the client actually understands. Codex CLI/Desktop declare
// Bash/Grep/Read/Write; opencode declares shell/read/write/grep style names;
// Claude Code declares Bash/Read/Write/Glob/Grep; generic OpenAI-compatible
// clients use read_file/write_file/search_files/shell_command.
type clientKind string

const (
	clientKindCodex     clientKind = "codex"
	clientKindCodexDesk clientKind = "codex_desktop"
	clientKindOpencode  clientKind = "opencode"
	clientKindClaude    clientKind = "claude"
	clientKindGeneric   clientKind = "generic"
)

// detectClientKind inspects the declared tool names to identify the calling
// client. Detection is name based only; no request headers are required so it
// works for every transport (SSE, polling, MCP).
func detectClientKind(declared []string) clientKind {
	lower := make([]string, 0, len(declared))
	for _, name := range declared {
		lower = append(lower, strings.ToLower(strings.TrimSpace(name)))
	}
	has := func(names ...string) bool {
		for _, n := range names {
			for _, d := range lower {
				if d == n {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("agent", "taskcreate", "workspace", "codex_custom_exec", "custom_exec", "askuserquestion"):
		return clientKindCodexDesk
	case has("todowrite", "todolist", "task"):
		return clientKindClaude
	case has("shell_command", "read_file", "write_file", "search_files", "apply_patch"):
		return clientKindOpencode
	case has("shell", "bash", "read", "write", "grep", "glob", "edit", "webfetch", "websearch"):
		return clientKindCodex
	case has("bash", "read", "write", "glob", "grep"):
		return clientKindClaude
	default:
		return clientKindGeneric
	}
}

// fallbackToolNamesByKind maps a client kind to the base tool names it
// understands. Injection only adds tools the client did not already declare,
// and only for intents the request actually expresses.
var fallbackToolNamesByKind = map[clientKind][]string{
	clientKindCodex:     {"Bash", "Read", "Write", "Grep"},
	clientKindCodexDesk: {"Bash", "Read", "Write", "Grep"},
	clientKindOpencode:  {"shell_command", "read_file", "write_file", "search_files"},
	clientKindClaude:    {"Bash", "Read", "Write", "Glob", "Grep"},
	clientKindGeneric:   {"shell_command", "read_file", "write_file", "search_files"},
}

// ToolTemplate describes a fallback base tool injected for clients that did
// not declare the tool themselves. Parameters keep full JSON Schema shape.
type ToolTemplate struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// clientToolTemplates provides structure-faithful schemas for the fallback
// base tools. The names are the ones each client declares; parameters keep
// full JSON Schema shape (types, enums, required) so the model can construct
// valid arguments.
var clientToolTemplates = map[string]ToolTemplate{
	"Bash": {
		Name:        "Bash",
		Description: "Run a shell command on the caller's machine and return its output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to execute"},
			},
			"required": []string{"command"},
		},
	},
	"Read": {
		Name:        "Read",
		Description: "Read a file from the caller's workspace at the given path.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Absolute or workspace-relative file path"},
			},
			"required": []string{"path"},
		},
	},
	"Write": {
		Name:        "Write",
		Description: "Write content to a file in the caller's workspace at the given path.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Absolute or workspace-relative file path"},
				"content": map[string]any{"type": "string", "description": "Content to write"},
			},
			"required": []string{"path", "content"},
		},
	},
	"Grep": {
		Name:        "Grep",
		Description: "Search for text or file patterns in the caller's workspace.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Search pattern or glob"},
				"path":    map[string]any{"type": "string", "description": "Directory to search"},
			},
			"required": []string{"pattern"},
		},
	},
	"Glob": {
		Name:        "Glob",
		Description: "List files in the caller's workspace matching a glob pattern.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Glob pattern relative to workspace"},
			},
			"required": []string{"pattern"},
		},
	},
	"shell_command": {
		Name:        "shell_command",
		Description: "Run a shell command on the caller's machine and return its output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to execute"},
			},
			"required": []string{"command"},
		},
	},
	"read_file": {
		Name:        "read_file",
		Description: "Read a file from the caller's workspace at the given path.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Absolute or workspace-relative file path"},
			},
			"required": []string{"path"},
		},
	},
	"write_file": {
		Name:        "write_file",
		Description: "Write content to a file in the caller's workspace at the given path.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Absolute or workspace-relative file path"},
				"content": map[string]any{"type": "string", "description": "Content to write"},
			},
			"required": []string{"path", "content"},
		},
	},
	"search_files": {
		Name:        "search_files",
		Description: "Search for files or content matching a query pattern.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query or glob pattern"},
			},
			"required": []string{"query"},
		},
	},
}

// kindForToolName maps a semantic tool kind to concrete tool names per client
// kind, used to decide which fallback tools satisfy an expressed intent.
var kindForToolName = map[clientToolKind][]string{
	clientToolKindRead:   {"read_file", "Read", "read"},
	clientToolKindWrite:  {"write_file", "Write", "write", "Edit", "apply_patch"},
	clientToolKindSearch: {"search_files", "Grep", "grep", "Glob", "glob"},
	clientToolKindShell:  {"shell_command", "Bash", "bash", "shell"},
}

// intentNeedsTool reports whether the matched intent kind has no declared tool
// covering it, meaning the gateway should inject a compatible fallback.
func intentNeedsTool(kind clientToolKind, declared []string) bool {
	for _, candidate := range kindForToolName[kind] {
		if containsString(declared, candidate) {
			return false
		}
	}
	return true
}

// intentSatisfiedByName reports whether a concrete tool name covers the given
// semantic intent kind (used to inject only the matching fallback tool).
func intentSatisfiedByName(kind clientToolKind, name string) bool {
	for _, candidate := range kindForToolName[kind] {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}

// injectFallbackTools adds structure-faithful base tools for the detected
// client when the request intent expresses a need the client did not declare.
// mcp__ tools and already-declared names are never duplicated.
func injectFallbackTools(kind clientKind, declared []string, intentKinds []clientToolKind) []chathub.Tool {
	if kind == clientKindGeneric || len(intentKinds) == 0 {
		return nil
	}
	// A client that only declared mcp__ tools has no local execution surface;
	// fallback injection is meaningless and mcp tools stay on the gateway.
	hasLocal := false
	for _, name := range declared {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "mcp__") {
			hasLocal = true
			break
		}
	}
	if !hasLocal {
		return nil
	}
	seen := make(map[string]bool, len(declared))
	for _, name := range declared {
		seen[strings.ToLower(strings.TrimSpace(name))] = true
	}
	added := make([]chathub.Tool, 0)
	for _, intent := range intentKinds {
		if !intentNeedsTool(intent, declared) {
			continue
		}
		for _, name := range fallbackToolNamesByKind[kind] {
			if !intentSatisfiedByName(intent, name) {
				continue
			}
			lower := strings.ToLower(name)
			if seen[lower] || strings.HasPrefix(lower, "mcp__") {
				continue
			}
			template, ok := clientToolTemplates[name]
			if !ok {
				continue
			}
			fn, _ := json.Marshal(template)
			added = append(added, chathub.Tool{Type: "function", Function: fn})
			seen[lower] = true
		}
	}
	return added
}

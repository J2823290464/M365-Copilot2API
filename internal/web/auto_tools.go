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

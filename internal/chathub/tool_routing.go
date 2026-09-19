package chathub

import (
	"encoding/json"
	"strings"
)

// localClientToolNames lists the tool names that execute on the caller's
// machine. Matching is case-insensitive: clients ship both lowercase names
// (read_file, shell_command) and PascalCase names (Read, Write, Grep, Bash),
// and a case-sensitive lookup silently classifies the PascalCase ones as
// cloud plugins. That misclassification drops them from the inlined tool
// protocol prompt, so the model answers "workspace not found" instead of
// calling the tool the client actually declared.
var localClientToolNames = map[string]struct{}{
	"bash":           {},
	"shell_command":  {},
	"powershell":     {},
	"terminal":       {},
	"read_file":      {},
	"write_file":     {},
	"search_file":    {},
	"search_files":   {},
	"read":           {},
	"write":          {},
	"edit":           {},
	"grep":           {},
	"glob":           {},
	"shell":          {},
	"sh":             {},
	"cmd":            {},
	"list_directory": {},
	"list_files":     {},
}

func toolName(tool Tool) string {
	var fn struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(tool.Function, &fn); err != nil {
		return ""
	}

	return strings.TrimSpace(fn.Name)
}

func toolNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))

	for _, tool := range tools {
		if name := toolName(tool); name != "" {
			names = append(names, name)
		}
	}

	return names
}

func isLocalClientTool(tool Tool) bool {
	name := toolName(tool)
	if name == "" {
		return false
	}

	_, ok := localClientToolNames[strings.ToLower(name)]
	return ok
}

func splitRoutedTools(tools []Tool) (
	nativeTools []Tool,
	promptTools []Tool,
) {
	for _, tool := range tools {
		if isLocalClientTool(tool) {
			promptTools = append(promptTools, tool)
			continue
		}

		nativeTools = append(nativeTools, tool)
	}

	return nativeTools, promptTools
}

func toolChoiceIsNone(choice any) bool {
	value, ok := choice.(string)
	if !ok {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(value), "none")
}

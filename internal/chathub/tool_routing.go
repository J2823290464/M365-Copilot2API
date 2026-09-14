package chathub

import (
	"encoding/json"
	"strings"
)

var localClientToolNames = map[string]struct{}{
	"bash":          {},
	"shell_command": {},
	"powershell":    {},
	"terminal":      {},
	"read_file":     {},
	"write_file":    {},
	"search_file":   {},
	"search_files":  {},
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

	_, ok := localClientToolNames[name]
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

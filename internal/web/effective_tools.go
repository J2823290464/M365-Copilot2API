package web

import (
	"encoding/json"
	"strings"

	"m365-copilot2api/internal/chathub"
	"m365-copilot2api/internal/mcp"
)

// effectiveClientTools 返回当前请求实际允许使用的工具。
//
//	default / auto_review → 仅使用客户端随请求声明的工具
//	full_access           → 客户端工具 + 服务端 MCP 注册表工具
//
// 工具名冲突时客户端声明优先，不会被注册表版本覆盖。
func effectiveClientTools(
	permission string,
	clientTools []chathub.Tool,
	registryTools []mcp.Tool,
) []chathub.Tool {
	result := append([]chathub.Tool(nil), clientTools...)

	if permission != "full_access" {
		return result
	}

	seen := make(map[string]struct{}, len(clientTools)+len(registryTools))
	for _, tool := range clientTools {
		name := chathubToolName(tool)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}

	for _, tool := range registryTools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			// 客户端声明优先，不覆盖客户端 schema
			continue
		}
		converted, ok := mcpToolToChatHubTool(tool)
		if !ok {
			continue
		}
		result = append(result, converted)
		seen[name] = struct{}{}
	}

	return result
}

// chathubToolName 从 chathub.Tool 的 Function 字段中提取工具名。
func chathubToolName(tool chathub.Tool) string {
	var function struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(tool.Function, &function); err != nil {
		return ""
	}
	return strings.TrimSpace(function.Name)
}

// mcpToolToChatHubTool 将一个 mcp.Tool 转换为 chathub.Tool。
func mcpToolToChatHubTool(tool mcp.Tool) (chathub.Tool, bool) {
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		return chathub.Tool{}, false
	}

	parameters := tool.InputSchema
	if parameters == nil {
		parameters = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	functionJSON, err := json.Marshal(map[string]any{
		"name":        name,
		"description": tool.Description,
		"parameters":  parameters,
	})
	if err != nil {
		return chathub.Tool{}, false
	}

	return chathub.Tool{
		Type:     "function",
		Function: functionJSON,
	}, true
}

// normalizeToolMaps 将 []chathub.Tool 转为服务端内部使用的 []map[string]any 格式。
func normalizeToolMaps(tools []chathub.Tool) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		var function map[string]any
		if err := json.Unmarshal(tool.Function, &function); err != nil {
			continue
		}
		name, _ := function["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		toolType := strings.TrimSpace(tool.Type)
		if toolType == "" {
			toolType = "function"
		}
		result = append(result, map[string]any{
			"type":     toolType,
			"function": function,
		})
	}
	return result
}

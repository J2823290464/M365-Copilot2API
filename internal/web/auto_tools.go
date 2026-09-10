package web

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"m365-copilot2api/internal/chathub"
)

type ToolTemplate struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type AutoToolRule struct {
	Patterns   []string
	MatchRegex bool
	ToolNames  []string
}

var autoToolRegistry = map[string]ToolTemplate{
	"write_file": {
		Name:        "write_file",
		Description: "Write content to a file at the specified path",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Absolute file path"},
				"content": map[string]any{"type": "string", "description": "Content to write"},
			},
			"required": []string{"path", "content"},
		},
	},
	"read_file": {
		Name:        "read_file",
		Description: "Read the contents of a file at the specified path",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Absolute file path"},
			},
			"required": []string{"path"},
		},
	},
	"search_files": {
		Name:        "search_files",
		Description: "Search for files or content matching a query pattern",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query or glob pattern"},
			},
			"required": []string{"query"},
		},
	},
	"bash": {
		Name:        "bash",
		Description: "Execute a bash/shell command",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to execute"},
			},
			"required": []string{"command"},
		},
	},
}

var defaultAutoToolRules = []AutoToolRule{
	{
		Patterns:  []string{"create file", "write file", "save file", "write a file", "new file", "创建文件", "新增文件", "写入文件", "写文件", "添加", "新建"},
		ToolNames: []string{"write_file"},
	},
	{
		Patterns:  []string{"read file", "view file", "open file", "read the file", "get file", "查看文件", "读取文件", "打开文件", "看文件", "看一下"},
		ToolNames: []string{"read_file"},
	},
	{
		Patterns:  []string{"search", "find", "grep", "locate", "search for", "look for", "搜索", "查找", "查询", "寻找", "找一下"},
		ToolNames: []string{"search_files"},
	},
	{
		Patterns:  []string{"run", "execute", "python", "npm", "go build", "docker", "bash", "运行", "执行", "启动"},
		ToolNames: []string{"bash"},
	},
	{
		Patterns:  []string{"edit", "modify", "replace", "patch", "update file", "编辑", "修改", "替换", "更新", "新增"},
		ToolNames: []string{"read_file", "write_file"},
	},
}

func autoInjectTools(prompt string, existingTools []chathub.Tool, rules []AutoToolRule) []chathub.Tool {
	if len(rules) == 0 {
		rules = defaultAutoToolRules
	}

	existingNames := make(map[string]bool)
	for _, t := range existingTools {
		var fn struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(t.Function, &fn) == nil && fn.Name != "" {
			existingNames[fn.Name] = true
		}
	}

	toAdd := make(map[string]bool)
	lowerPrompt := strings.ToLower(prompt)
	matchedRules := 0
	for _, rule := range rules {
		matched := false
		for _, pattern := range rule.Patterns {
			if rule.MatchRegex {
				if re, err := regexp.Compile("(?i)" + pattern); err == nil && re.MatchString(prompt) {
					matched = true
					break
				}
			} else {
				if strings.Contains(lowerPrompt, strings.ToLower(pattern)) {
					matched = true
					break
				}
			}
		}
		if matched {
			matchedRules++
			for _, name := range rule.ToolNames {
				if !existingNames[name] {
					toAdd[name] = true
				}
			}
		}
	}

	if len(toAdd) == 0 {
		log.Printf("[auto-tools] no tools injected. matched_rules=%d", matchedRules)
		return existingTools
	}

	// Build ordered list of injected tool names for logging.
	injected := make([]string, 0, len(toAdd))
	result := make([]chathub.Tool, 0, len(existingTools)+len(toAdd))
	result = append(result, existingTools...)
	for name := range toAdd {
		tmpl, ok := autoToolRegistry[name]
		if !ok {
			continue
		}
		fnData := map[string]any{
			"name":        tmpl.Name,
			"description": tmpl.Description,
			"parameters":  tmpl.Parameters,
		}
		fnJSON, _ := json.Marshal(fnData)
		result = append(result, chathub.Tool{
			Type:     "function",
			Function: fnJSON,
		})
		injected = append(injected, name)
	}
	log.Printf("[auto-tools] injected tools=%v existing=%d new=%d matched_rules=%d", injected, len(existingTools), len(injected), matchedRules)
	return result
}

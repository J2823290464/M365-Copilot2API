package chathub

import (
	"encoding/json"
	"fmt"
	"strings"
)

// toolProtocolPrompt follows the community-compatible M365 convention:
// definitions are wrapped in <tools>, and calls are emitted as a fenced block
// whose info string is the exact tool name.
func toolProtocolPrompt(text string, tools []Tool, choice any) string {
	if len(tools) == 0 || toolChoiceIsNone(choice) {
		return text
	}
	defs := make([]string, 0, len(tools))

	for _, tool := range tools {
		var fn struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		}

		if err := json.Unmarshal(tool.Function, &fn); err != nil {
			continue
		}

		if strings.TrimSpace(fn.Name) == "" {
			continue
		}

		parameters := strings.TrimSpace(string(fn.Parameters))
		if parameters == "" || parameters == "null" {
			parameters = "{}"
		}

		defs = append(defs, fmt.Sprintf(
			"%s - %s\n```%s\n%s\n```",
			fn.Name,
			fn.Description,
			fn.Name,
			parameters,
		))
	}

	if len(defs) == 0 {
		return text
	}

	return fmt.Sprintf(`You are an execution agent operating through client tools on the caller's machine.
    The tools listed below are real, active, and callable for this request. Use the exact tool names shown in the tool definitions. Do not invent aliases or substitute another tool name.
    Filesystem and command tools execute on the caller's machine, not in the cloud model sandbox. Local paths mentioned in the request, conversation context, or client metadata must be accessed through the available client tools. Do not assume that a path is unavailable merely because it is not visible in the cloud model sandbox.
    Do not inspect or infer local workspace availability from /mnt/data or from the cloud execution environment.
    If the request requires inspecting, searching, modifying, testing, or verifying local files, you must attempt an appropriate available client tool before claiming that the path or workspace is inaccessible.
    If a client tool returns an actual permission, path, approval, or execution error, report that concrete error accurately. Do not claim that access succeeded when the tool failed.
    When calling a tool, emit exactly one fenced code block whose info string is the exact tool name and whose body is valid JSON matching that tool's schema.
    <tools>
    %s
    </tools>
    User request:
    %s`, strings.Join(defs, "\n\n"), text)
}

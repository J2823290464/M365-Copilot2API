# 客户端工具透传与兼容注入 —— 需求与决策记录

> 用途：防止后续改动无意中回归本功能。改动工具路由相关代码前，先读本文件。

## 一、需求（用户明确，2026-09-17/18）

1. **客户端声明的工具应当全部透传给云端**，让云端模型知道"有哪些工具可调"。
   - 背景问题：此前 `autoSelectClientTools` 把客户端 60 个工具过滤成 2~4 个，云端不知道本地工具存在，
     于是不会主动要求中转调用客户端工具，导致"找不到项目路径、无法修改文件"等错误。
2. **透传做成页面开关**，不是无条件开启（语义 2026-09-18 修订）。
   - 开关：设置页 "Pass-through client tools"，对应 `M365_PASS_THROUGH_CLIENT_TOOLS`（默认 false）。
   - 开关开：客户端声明工具**全量透传、原样保留**，不过滤、不注入任何基础工具。
   - 开关关：关键词过滤（`autoSelectClientTools`）+ 按意图注入客户端兼容的缺失基础工具。
3. **`mcp__` 前缀工具一律由 MCP 网关承载**，不进 `<tools>` 文本，不被客户端注入逻辑覆盖。
4. **兜底注入按客户端 + 意图**，不是统一一套工具：
   - 客户端类型识别：Codex CLI、Codex Desktop、opencode、Claude Code、generic（按声明工具名推断）。
   - 只有"意图命中但客户端未声明对应工具"时才注入。
   - 纯 `mcp__` 客户端不注入基础工具（无本地执行面）。

## 二、实现位置

- 开关字段：`internal/web/settings.go` `PassThroughClientTools`（`M365_PASS_THROUGH_CLIENT_TOOLS`，默认 false）。
- 开关语义入口：`internal/web/auto_tools.go` `routeClientTools(passThrough, prompt, tools)`，server.go openaiChat 工具构建段调用。
- 客户端识别：`internal/web/auto_tools.go` `detectClientKind`。
- 兜底注入：`internal/web/auto_tools.go` `injectFallbackTools` + `clientToolTemplates`。
- 前端开关：`web/index.html` 设置页 `setPassThroughClientTools`。

## 三、客户端兼容矩阵（detectClientKind 判定依据）

| 客户端 | 特征工具名（声明中出现即命中） | 注入的基础工具 |
| --- | --- | --- |
| Codex Desktop | agent / taskcreate / workspace / codex_custom_exec / custom_exec / askuserquestion | Bash, Read, Write, Grep |
| Claude Code | todowrite / todolist / task | Bash, Read, Write, Glob, Grep |
| opencode | shell_command / read_file / write_file / search_files / apply_patch | shell_command, read_file, write_file, search_files |
| Codex CLI | shell / bash / read / write / grep / glob / edit / webfetch / websearch | Bash, Read, Write, Grep |
| generic | 其余 | 不注入 |

注意：`TodoWrite` 归一化后是 `todowrite`；Claude 分支必须排在 Codex 之前，否则 `Bash/Read/Write/Grep` 会先命中 Codex。

## 四、MCP 网关的作用（不是简单的"分流"）

MCP 网关是**中转内置的 MCP 服务端**（`/v1/mcp/sse` + `/v1/mcp/message` + `/v1/mcp/tools`），作用：

1. **承载**：所有注册进 `GlobalToolRegistry` 的工具（含客户端经 `MergeTools` 注册的 `mcp__*` 工具）。
2. **暴露**：云端通过 ChatHub 的 `mcp-gateway` 插件（`Transport: mcp` + `TransportUrl`）发现这些工具，
   与"本地客户端工具走 `<tools>` 文本"是两条独立通道。
3. **转发**：云端发起 MCP 调用 → 网关 JSON-RPC 处理 → 由 ToolProvider（MCP 客户端/静态注册表）执行 → 结果回传云端。

即：本地工具走 `<tools>` 文本协议，`mcp__*` 工具走 MCP 网关协议。二者不混。

## 五、回归注意

- 透传分支（开）：body.Tools 必须原样保留全量，**不得**调用 autoSelectClientTools，**不得**注入。
- 过滤分支（关）：autoSelectClientTools 选出子集后，再按意图注入缺失基础工具；注入必须跳过已声明工具和 `mcp__` 前缀；纯 mcp 客户端不注入。
- 开关默认 false（=过滤+注入），改动默认行为会影响现有用户。
- 相关测试：`internal/web/auto_tools_test.go`（TestDetectClientKind、TestInjectFallback*）。
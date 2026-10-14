// Package delmresearch defines the Phase 0 read-only research tasks for
// comparing DeLM-style coordination with single-agent and existing subagent
// baselines. It is a development tool, not a runtime package.
//
// Every task is answered by reading this repository at commit 676a0a2. A fact
// counts as covered when its Expect text appears in the final answer,
// compared case-insensitively. Anchor is the literal that proves the fact in
// File. TestAnswerKeyAnchors fails when the code moves, so refresh the affected
// task before comparing results across commits.
package delmresearch

import "strings"

// Fact is one checkable item in a task's answer key.
type Fact struct {
	Expect string // text the final answer must contain
	File   string // repo-relative path that proves the fact
	Anchor string // literal that must appear in File
}

// Task is one read-only research question with its answer key.
type Task struct {
	ID     string
	Prompt string
	Facts  []Fact
}

// Score returns how many facts the answer covers, and the total fact count.
func (t Task) Score(answer string) (hit, total int) {
	lower := strings.ToLower(answer)
	for _, f := range t.Facts {
		if strings.Contains(lower, strings.ToLower(f.Expect)) {
			hit++
		}
	}
	return hit, len(t.Facts)
}

// Pass reports whether the answer covers every fact.
func (t Task) Pass(answer string) bool {
	hit, total := t.Score(answer)
	return total > 0 && hit == total
}

// Tasks is the Phase 0 set. A prompt must not contain its own Expect text;
// TestPromptsDoNotLeakAnswers enforces that.
var Tasks = []Task{
	{
		ID: "t01_background_cancel",
		Prompt: "后台子代理（background）是怎样被取消的？请回答：" +
			"1) 取消入口的函数名；" +
			"2) 运行中的后台子代理被登记在哪个 map 字段里，键是什么；" +
			"3) 取消时写入会话元数据的 status 值；" +
			"4) 后台子代理的 context 是从哪里派生的，是否继承父请求的取消。",
		Facts: []Fact{
			{Expect: "CancelBackgroundSubagent", File: "internal/tasks/subagent.go", Anchor: "func (r *Runner) CancelBackgroundSubagent"},
			{Expect: "backgroundCancels", File: "internal/tasks/subagent.go", Anchor: "r.backgroundCancels[strings.TrimSpace(sessionID)] = cancel"},
			{Expect: "cancelling", File: "internal/tasks/subagent.go", Anchor: `"cancelling"`},
			{Expect: "context.Background", File: "internal/tasks/subagent.go", Anchor: "context.WithCancel(context.Background())"},
		},
	},
	{
		ID: "t02_pending_injection",
		Prompt: "运行中的 agent 如何在一轮对话中途接收新消息？请回答：" +
			"1) 追加待注入消息的方法名（所在结构体为 activeTurnState）；" +
			"2) 取出待注入消息的方法名；" +
			"3) 取出的消息被追加到哪个日志对象（写出变量和字段名）；" +
			"4) 取出动作位于哪个源文件。",
		Facts: []Fact{
			{Expect: "appendPending", File: "internal/agent/agent.go", Anchor: "func (s *activeTurnState) appendPending"},
			{Expect: "drainPending", File: "internal/agent/agent.go", Anchor: "func (s *activeTurnState) drainPending"},
			{Expect: "rt.Log", File: "internal/agent/turn_loop.go", Anchor: "rt.Log.Append(msg)"},
			{Expect: "turn_loop.go", File: "internal/agent/turn_loop.go", Anchor: "turnState.drainPending()"},
		},
	},
	{
		ID:     "t03_child_excluded_tools",
		Prompt: "子代理（child agent）被禁止使用哪些工具？请列出完整名单，并说明这份名单定义在哪个文件、哪个变量里。",
		Facts: []Fact{
			{Expect: "excludedChildTools", File: "internal/tasks/registry.go", Anchor: "var excludedChildTools"},
			{Expect: "registry.go", File: "internal/tasks/registry.go", Anchor: "var excludedChildTools"},
			{Expect: "parallel_reason", File: "internal/tasks/registry.go", Anchor: `"parallel_reason"`},
			{Expect: "spawn_subagent", File: "internal/tasks/registry.go", Anchor: `"spawn_subagent"`},
			{Expect: "request_user_input", File: "internal/tasks/registry.go", Anchor: `"request_user_input"`},
			{Expect: "update_plan", File: "internal/tasks/registry.go", Anchor: `"update_plan"`},
			{Expect: "todo_add", File: "internal/tasks/registry.go", Anchor: `"todo_add"`},
			{Expect: "todo_update", File: "internal/tasks/registry.go", Anchor: `"todo_update"`},
			{Expect: "todo_remove", File: "internal/tasks/registry.go", Anchor: `"todo_remove"`},
			{Expect: "todo_clear_done", File: "internal/tasks/registry.go", Anchor: `"todo_clear_done"`},
			{Expect: "todo_list", File: "internal/tasks/registry.go", Anchor: `"todo_list"`},
		},
	},
	{
		ID: "t04_isolation_default",
		Prompt: "子代理默认是否在独立的 git worktree 中运行？请回答：" +
			"1) 默认取值是什么；" +
			"2) 决定取值的规范化函数名；" +
			"3) 开启隔离应在 AgentDefinition 的哪个 JSON 字段上设置；" +
			"4) 开启后 worktree 被创建在仓库的哪个子目录下。",
		Facts: []Fact{
			{Expect: "none", File: "internal/tasks/agent_definition.go", Anchor: `= "none"`},
			{Expect: "normalizeAgentIsolation", File: "internal/tasks/agent_definition.go", Anchor: "func normalizeAgentIsolation"},
			{Expect: "isolation", File: "internal/tasks/agent_definition.go", Anchor: `json:"isolation,omitempty"`},
			{Expect: "worktrees", File: "internal/worktree/worktree.go", Anchor: `".whale/worktrees/"`},
		},
	},
	{
		ID:     "t05_config_files",
		Prompt: "whale 有哪些 TOML 配置文件？请回答：1) 全局配置的文件名；2) 项目级配置的完整相对路径（相对仓库根目录）；3) 项目级本地覆盖配置（不提交到仓库）的完整相对路径。",
		Facts: []Fact{
			{Expect: "config.toml", File: "internal/app/config_file.go", Anchor: `"config.toml"`},
			{Expect: "config.local.toml", File: "internal/app/config_file.go", Anchor: `"config.local.toml"`},
			{Expect: ".whale/config.toml", File: "internal/app/config_file.go", Anchor: `".whale", ConfigFileName`},
			{Expect: ".whale/config.local.toml", File: "internal/app/config_file.go", Anchor: `".whale", LocalConfigFileName`},
		},
	},
	{
		ID: "t06_max_parallel_subagents",
		Prompt: "子代理的并行数上限由什么决定？请回答：" +
			"1) 默认上限常量的名字和数值；" +
			"2) 决定默认值的函数名；" +
			"3) 用户在 TOML 中调整该值用的键名，以及它所在的顶层段名。",
		Facts: []Fact{
			{Expect: "defaultMaxParallelSubagentCap", File: "internal/agent/agent.go", Anchor: "const defaultMaxParallelSubagentCap"},
			{Expect: "128", File: "internal/agent/agent.go", Anchor: "defaultMaxParallelSubagentCap = 128"},
			{Expect: "defaultMaxParallelSubagents", File: "internal/agent/agent.go", Anchor: "func defaultMaxParallelSubagents"},
			{Expect: "max_parallel_subagents", File: "internal/app/config_file.go", Anchor: `toml:"max_parallel_subagents,omitempty"`},
			{Expect: "tasks", File: "internal/app/config_file.go", Anchor: `toml:"tasks,omitempty"`},
		},
	},
	{
		ID: "t07_usage_telemetry",
		Prompt: "token 用量与缓存命中数据是如何持久化的？请回答：" +
			"1) 记录结构体的名字及所在文件；" +
			"2) 写入函数的名字；" +
			"3) 默认目录（相对用户主目录的路径）；" +
			"4) 子代理的用量记录按哪个字段决定写入哪个文件。",
		Facts: []Fact{
			{Expect: "UsageRecord", File: "internal/telemetry/usage_log.go", Anchor: "type UsageRecord struct"},
			{Expect: "AppendUsage", File: "internal/telemetry/usage_log.go", Anchor: "func AppendUsage("},
			{Expect: ".whale/usage", File: "internal/telemetry/usage_log.go", Anchor: `".whale", "usage"`},
			{Expect: "ParentSessionID", File: "internal/telemetry/usage_log.go", Anchor: `if pid := strings.TrimSpace(rec.ParentSessionID); pid != ""`},
		},
	},
	{
		ID: "t08_workflow_events",
		Prompt: "workflow 运行的事件日志存放在哪里？请回答：" +
			"1) 相对于单次 run 目录的文件名；" +
			"2) 事件结构体的名字及所在文件；" +
			"3) 恢复（resume）时用来匹配已完成调用的字段名。",
		Facts: []Fact{
			{Expect: "events.jsonl", File: "internal/workflow/store.go", Anchor: `"events.jsonl"`},
			{Expect: "RunEvent", File: "internal/workflow/types.go", Anchor: "type RunEvent struct"},
			{Expect: "CallKey", File: "internal/workflow/resume.go", Anchor: "CallKey"},
		},
	},
	{
		ID: "t09_approval_cache",
		Prompt: "会话的审批记录存放在哪里？请回答：" +
			"1) 审批文件名在会话 ID 后面加的后缀；" +
			"2) 会话消息文件使用的后缀；" +
			"3) 授予审批的接口方法名（所在接口为 ApprovalStore）；" +
			"4) 拼接文件名前对会话 ID 做清洗的函数名。",
		Facts: []Fact{
			{Expect: ".approvals.json", File: "internal/store/jsonl.go", Anchor: `".approvals.json"`},
			{Expect: ".jsonl", File: "internal/store/jsonl.go", Anchor: `".jsonl"`},
			{Expect: "GrantApproval", File: "internal/store/store.go", Anchor: "GrantApproval(ctx context.Context"},
			{Expect: "SanitizeSessionID", File: "internal/store/jsonl.go", Anchor: `core.SanitizeSessionID(sessionID)+".approvals.json"`},
		},
	},
	{
		ID: "t10_subagent_result",
		Prompt: "子代理工具返回给父代理的结果包含哪些字段？请列出字段名。" +
			"另外，模型能否在调用参数中设置子代理的工具调用上限？" +
			"如果不能，请说明代码在哪个文件、哪两行把它清零。",
		Facts: []Fact{
			{Expect: "report", File: "internal/tasks/tools.go", Anchor: `"report":`},
			{Expect: "summary", File: "internal/tasks/tools.go", Anchor: `"summary":`},
			{Expect: "status", File: "internal/tasks/tools.go", Anchor: `"status":`},
			{Expect: "MaxToolCalls", File: "internal/tasks/tools.go", Anchor: "req.MaxToolCalls = 0"},
			{Expect: "MaxToolIters", File: "internal/tasks/tools.go", Anchor: "req.MaxToolIters = 0"},
		},
	},
}

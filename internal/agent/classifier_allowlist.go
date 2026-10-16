package agent

// classifierAllowlist is the set of tool names that are always safe and do not
// need classifier review. Translated from Claude Code's SAFE_YOLO_ALLOWLISTED_TOOLS
// in classifierDecision.ts.
//
// Only read-only, metadata-only, or UI-only tools are included. Mutating tools
// (write, edit, multi_edit, shell_run, web_fetch) are NEVER in this list — they
// go through the classifier.
var classifierAllowlist = map[string]bool{
	// File read and inspection
	"read_file":    true,
	"list_dir":     true,
	"grep":         true,
	"glob":         true,
	"search_files": true,

	// Shell polling (read-only, waits for background task)
	"shell_wait": true,

	// Memory operations (read-only, metadata only)
	"recall_memory": true,

	// Todo management (metadata only — no file mutation)
	"todo_list": true,

	// Goal inspection (read-only metadata)
	"get_goal": true,

	// Subagent inspection (read-only status check)
	"subagent_status": true,

	// Model-side reasoning (no tool execution, purely reasoning)
	"parallel_reason": true,

	// User interaction (no side effects outside the agent loop)
	"request_user_input": true,

	// Plan inspection (metadata only)
	"update_plan": true,
}

// isClassifierAllowlisted returns true when a tool call should skip classifier
// review entirely. These tools are provably safe: they are read-only, operate
// only on metadata, or are pure model-side operations.
func isClassifierAllowlisted(toolName string) bool {
	return classifierAllowlist[toolName]
}

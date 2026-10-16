package agent

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/usewhale/whale/internal/core"
)

func TestClassifierReview_FailClosed(t *testing.T) {
	// Test fail-closed: with no API key, classifier should block (fail-safe)
	cfg := ClassifierConfig{
		Enabled:   true,
		TimeoutMS: 5000,
	}
	c := NewClassifier(cfg)
	// Override API key to empty to force failure
	c.apiKey = ""

	messages := []core.Message{
		{Role: core.RoleUser, Text: "test"},
	}
	call := core.ToolCall{ID: "test-fail", Name: "shell_run", Input: `{"command":"ls"}`}

	review := c.Review(context.Background(), messages, call, "/tmp/test")
	if review.Result.Decision != ClassifierDecisionBlock {
		t.Errorf("no API key should fail-closed (block), got %s: %s", review.Result.Decision, review.Result.Reason)
	}
}

func TestClassifierReview_Allowlist(t *testing.T) {
	allowlistTests := []string{"read_file", "list_dir", "grep", "glob", "shell_wait", "recall_memory", "todo_list", "get_goal", "subagent_status", "parallel_reason", "request_user_input", "update_plan"}
	for _, name := range allowlistTests {
		if !isClassifierAllowlisted(name) {
			t.Errorf("expected %s to be allowlisted", name)
		}
	}

	// Mutating tools should NOT be allowlisted
	mutatingTools := []string{"shell_run", "write", "edit", "multi_edit", "web_fetch", "web_search", "remember", "forget"}
	for _, name := range mutatingTools {
		if isClassifierAllowlisted(name) {
			t.Errorf("expected %s to NOT be allowlisted", name)
		}
	}
}

func TestBuildClassifierTranscript(t *testing.T) {
	messages := []core.Message{
		{Role: core.RoleUser, Text: "帮我看看项目"},
		{Role: core.RoleAssistant, ToolCalls: []core.ToolCall{
			{ID: "c1", Name: "shell_run", Input: `{"command":"ls -la"}`},
		}},
		{Role: core.RoleUser, Text: "删除临时文件"},
	}
	action := core.ToolCall{ID: "c2", Name: "shell_run", Input: `{"command":"rm -rf /tmp/test"}`}

	transcript := buildClassifierTranscript(messages, action)

	// Should contain user messages
	if !strings.Contains(transcript, "帮我看看项目") {
		t.Error("transcript should contain first user message")
	}
	if !strings.Contains(transcript, "删除临时文件") {
		t.Error("transcript should contain second user message")
	}
	// Should contain tool call
	if !strings.Contains(transcript, "ls -la") {
		t.Error("transcript should contain ls command")
	}
	// Should contain the action being reviewed
	if !strings.Contains(transcript, "rm -rf /tmp/test") {
		t.Error("transcript should contain the action being reviewed")
	}
	// Assistant text should be excluded (not present in test data)
	// The transcript should NOT contain raw assistant messages
}

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker()
	turnID := "test-turn"

	// Allow → reset consecutive
	cb.RecordNonDenial(turnID)
	if cb.IsInterrupted(turnID) {
		t.Error("should not be interrupted after allow")
	}

	// 3 consecutive denials → interrupt
	if cb.RecordDenial(turnID) {
		t.Error("should not interrupt on first denial")
	}
	if cb.RecordDenial(turnID) {
		t.Error("should not interrupt on second denial")
	}
	if !cb.RecordDenial(turnID) {
		t.Error("should interrupt on third consecutive denial")
	}
	if !cb.IsInterrupted(turnID) {
		t.Error("should be interrupted")
	}

	// New turn should be clean
	cb.ClearTurn(turnID)
	if cb.IsInterrupted(turnID) {
		t.Error("should not be interrupted after clear")
	}

	// Test consecutive denials counter resets properly after non-denial
	turn2 := "test-turn-2"
	// First denial
	if cb.RecordDenial(turn2) {
		t.Error("should not interrupt on first denial for new turn")
	}
	// Second denial
	if cb.RecordDenial(turn2) {
		t.Error("should not interrupt on second denial")
	}
	// Non-denial resets the counter
	cb.RecordNonDenial(turn2)
	// After reset, two more denials should not trigger
	if cb.RecordDenial(turn2) {
		t.Error("should not interrupt after reset + 1 denial")
	}
	if cb.RecordDenial(turn2) {
		t.Error("should not interrupt after reset + 2 denials")
	}
	// Third consecutive after reset → interrupt
	if !cb.RecordDenial(turn2) {
		t.Error("should interrupt on third consecutive denial after reset")
	}

	// Test that consecutive resets after a non-denial
	turn3 := "test-turn-3"
	cb.RecordDenial(turn3)
	cb.RecordDenial(turn3)
	if cb.IsInterrupted(turn3) {
		t.Error("should not be interrupted after 2 denials")
	}
	cb.RecordNonDenial(turn3) // reset
	// After reset, 3 more denials should trigger
	cb.RecordDenial(turn3)
	cb.RecordDenial(turn3)
	cb.RecordDenial(turn3)
	if !cb.IsInterrupted(turn3) {
		t.Error("should be interrupted after 3 consecutive denials post-reset")
	}
}

func TestCompactToolInput(t *testing.T) {
	tests := []struct {
		name     string
		tc       core.ToolCall
		contains string
		excludes string
	}{
		{
			name:     "shell command extraction",
			tc:       core.ToolCall{Name: "shell_run", Input: `{"command":"go test ./..."}`},
			contains: "go test ./...",
		},
		{
			name:     "write file preview",
			tc:       core.ToolCall{Name: "write", Input: `{"file_path":"main.go","content":"package main\n\nfunc main() {\n\tprintln(\"hello\")\n}"}`},
			contains: "file=main.go",
		},
		{
			name:     "multi_edit preview",
			tc:       core.ToolCall{Name: "multi_edit", Input: `{"file_path":"go.mod","edits":[{"search":"old","replace":"new"}]}`},
			contains: "file=go.mod",
		},
		{
			name:     "default truncation",
			tc:       core.ToolCall{Name: "unknown_tool", Input: `{"key":"value"}`},
			contains: `{"key":"value"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compactToolInput(tt.tc)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain %q, got %q", tt.contains, result)
			}
			if tt.excludes != "" && strings.Contains(result, tt.excludes) {
				t.Errorf("expected result to exclude %q, got %q", tt.excludes, result)
			}
		})
	}
}

// TestNewClassifierBaseURLPrecedence pins the endpoint precedence: an explicit
// config value (the app passes its resolved api.base_url) beats the env
// override, which beats the public default. Auto-review must not bypass a
// configured gateway/proxy: the default endpoint is unreachable there, so every
// review would fail closed and block writes.
func TestNewClassifierBaseURLPrecedence(t *testing.T) {
	t.Setenv("DEEPSEEK_BASE_URL", "https://env.example/v1/")
	if got := NewClassifier(ClassifierConfig{BaseURL: "https://cfg.example/v1/"}).baseURL; got != "https://cfg.example/v1" {
		t.Fatalf("configured base URL must win, got %q", got)
	}
	if got := NewClassifier(ClassifierConfig{}).baseURL; got != "https://env.example/v1" {
		t.Fatalf("env base URL must be the fallback, got %q", got)
	}
	t.Setenv("DEEPSEEK_BASE_URL", "")
	if got := NewClassifier(ClassifierConfig{}).baseURL; got != defaultClassifierBaseURL {
		t.Fatalf("public default expected, got %q", got)
	}
}

// TestTruncateStringKeepsRuneBoundary guards the classifier transcript against
// being cut mid-rune: an invalid slice JSON-encodes as U+FFFD and hands the
// classifier a mangled command.
func TestTruncateStringKeepsRuneBoundary(t *testing.T) {
	got := truncateString(strings.Repeat("中", 10), 7)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated string is not valid UTF-8: %q", got)
	}
	if want := strings.Repeat("中", 2) + "..."; got != want {
		t.Fatalf("truncateString = %q, want %q", got, want)
	}
	if got, want := truncateString("abc", 3), "abc"; got != want {
		t.Fatalf("under the limit must pass through: got %q, want %q", got, want)
	}
	if got, want := truncateString("中文", 0), "..."; got != want {
		t.Fatalf("non-positive limit must keep only the ellipsis: got %q, want %q", got, want)
	}
}

// TestDetectOSReportsHostOS documents that the OS hint handed to the classifier
// reflects the host it runs on, not a hardcoded platform.
func TestDetectOSReportsHostOS(t *testing.T) {
	if got := detectOS(); got != runtime.GOOS {
		t.Fatalf("detectOS = %q, want %q", got, runtime.GOOS)
	}
}

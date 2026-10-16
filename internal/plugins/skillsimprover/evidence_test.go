package skillsimprover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/agent"
)

// TestEvidenceRedactsSecretsBeforeTruncating pins the order: a secret is
// scrubbed while it is whole, so a summary that is cut later cannot leak a
// key prefix (M1-29).
func TestEvidenceRedactsSecretsBeforeTruncating(t *testing.T) {
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, workspace)

	// The key sits inside the cap: it must be replaced, not kept.
	inside, err := store.AppendEvidence(Evidence{
		Kind: "tool-failure", SessionID: "s1", Skill: "demo",
		Summary: "auth failed with sk-abcdefghijklmnop while calling the API",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(inside.Summary, "sk-abc") || !strings.Contains(inside.Summary, "[redacted]") {
		t.Fatalf("secret inside the cap should be replaced, got %q", inside.Summary)
	}

	// A straddling secret must be redacted before the cut, or the truncation
	// would leave a partial key behind.
	straddle, err := store.AppendEvidence(Evidence{
		Kind: "tool-failure", SessionID: "s1", Skill: "demo",
		Summary: strings.Repeat("x", maxEvidenceSummaryLen-5) + "sk-abcdefghijkl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(straddle.Summary, "sk-a") {
		t.Fatalf("a straddling secret leaked its prefix: %q", straddle.Summary)
	}
	if len([]rune(straddle.Summary)) > maxEvidenceSummaryLen+len("...[truncated]") {
		t.Fatalf("summary exceeds the cap: %d runes", len([]rune(straddle.Summary)))
	}

	// The same order applies to key=value credentials.
	kv, err := store.AppendEvidence(Evidence{
		Kind: "tool-failure", SessionID: "s1", Skill: "demo",
		Summary: "env dump: token:ghp_abcdefghijklmno password=hunter2hunter2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(kv.Summary, "ghp_") || strings.Contains(kv.Summary, "hunter2") {
		t.Fatalf("key=value secrets should be redacted, got %q", kv.Summary)
	}
}

func TestToolFailureEvidencePrefersRenderedCause(t *testing.T) {
	payload := agent.HookPayload{
		Event:         agent.HookEventPostToolUse,
		SessionID:     "s1",
		ToolName:      "shell_run",
		ToolResult:    "error (exec_failed): tests failed: 3 of 41 cases\nstderr:\n--- FAIL: TestX",
		ToolOutcome:   "failure",
		ToolErrorCode: "exec_failed",
	}
	ev, ok := toolFailureEvidence(payload)
	if !ok {
		t.Fatal("expected failure evidence")
	}
	// The human-readable rendered line is the cause; the bare code stays in its
	// own field.
	if ev.Summary != "error (exec_failed): tests failed: 3 of 41 cases" {
		t.Fatalf("summary should be the first rendered line, got %q", ev.Summary)
	}
	if ev.ErrorCode != "exec_failed" {
		t.Fatalf("failure code missing: %q", ev.ErrorCode)
	}

	// Success outcomes never produce evidence.
	payload.ToolOutcome = "success"
	if _, ok := toolFailureEvidence(payload); ok {
		t.Fatal("success outcome must not produce failure evidence")
	}
}

// TestEvidenceRecordsOnlyTheMinimizedFields pins the on-disk shape of an
// evidence record: the whitelist is the privacy boundary itself, so a new
// field must fail here before it can reach a user's disk (M1-26, M1-27, M1-28).
func TestEvidenceRecordsOnlyTheMinimizedFields(t *testing.T) {
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, workspace)

	allowed := map[string]bool{
		"id": true, "created_at": true, "kind": true,
		"session_id": true, "skill": true, "error_code": true, "summary": true,
	}

	if _, err := store.AppendEvidence(Evidence{
		Kind:      "user-feedback",
		SessionID: "s1",
		Skill:     "demo",
		Summary:   "下次 $demo 要更具体\n第二行不该落盘",
	}); err != nil {
		t.Fatalf("append feedback evidence: %v", err)
	}
	if _, err := store.AppendEvidence(Evidence{
		Kind:      "tool-failure",
		SessionID: "s1",
		Skill:     "demo",
		ErrorCode: "exec_failed",
		Summary:   strings.Repeat("失败原因很长 ", 100),
	}); err != nil {
		t.Fatalf("append failure evidence: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dataDir, "evidence.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two records, got %d", len(lines))
	}
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal record: %v", err)
		}
		for key := range rec {
			if !allowed[key] {
				t.Errorf("record carries %q, which is outside the minimized field set", key)
			}
		}
	}

	evs, err := store.ListEvidence("demo", 10)
	if err != nil || len(evs) != 2 {
		t.Fatalf("list evidence len=%d err=%v", len(evs), err)
	}
	for _, ev := range evs {
		if strings.Contains(ev.Summary, "\n") {
			t.Errorf("summary keeps more than one line: %q", ev.Summary)
		}
		switch ev.Kind {
		case "user-feedback":
			if ev.Summary != "下次 $demo 要更具体" {
				t.Errorf("feedback summary should be the first line only, got %q", ev.Summary)
			}
		case "tool-failure":
			if !strings.Contains(ev.Summary, "...[truncated]") || len([]rune(ev.Summary)) > maxEvidenceSummaryLen+len("...[truncated]") {
				t.Errorf("long failure summary should be truncated to the cap, got %q", ev.Summary)
			}
			if ev.ErrorCode != "exec_failed" {
				t.Errorf("failure code should survive minimization, got %q", ev.ErrorCode)
			}
		}
	}
}

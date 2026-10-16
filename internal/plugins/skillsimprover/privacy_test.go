package skillsimprover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/agent"
)

// M1-30: a failed evidence write stays visible in status instead of vanishing.
func TestEvidenceWriteFailureIsVisibleInStatus(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the file mode that makes the write fail")
	}
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dataDir, evidenceFileName)
	// A read-only log is still readable, so the failure shows up in status
	// rather than turning the whole status call into an error.
	if err := os.WriteFile(logPath, nil, 0o400); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dataDir, workspace)

	store.RecordEvidence(Evidence{Kind: "tool-failure", SessionID: "s1", Skill: "demo", Summary: "boom"})

	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.WriteError == "" {
		t.Fatal("a failed evidence write must show up in status")
	}

	// Once the log is writable again the failure is cleared, not sticky.
	if err := os.Remove(filepath.Join(dataDir, evidenceFileName)); err != nil {
		t.Fatal(err)
	}
	store.RecordEvidence(Evidence{Kind: "tool-failure", SessionID: "s1", Skill: "demo", Summary: "boom"})
	status, err = store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.WriteError != "" || status.EvidenceCount != 1 {
		t.Fatalf("status should recover after a successful write: %+v", status)
	}
}

// M1-31: evidence can be listed and filtered by skill.
func TestEvidenceListingFiltersBySkill(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), t.TempDir())
	for _, skill := range []string{"demo", "other", "demo"} {
		if _, err := store.AppendEvidence(Evidence{Kind: "tool-failure", SessionID: "s1", Skill: skill, Summary: skill + " failed"}); err != nil {
			t.Fatal(err)
		}
	}

	demo, err := store.ListEvidence("demo", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(demo) != 2 {
		t.Fatalf("expected the two demo records, got %d", len(demo))
	}
	for _, ev := range demo {
		if ev.Skill != "demo" {
			t.Fatalf("filter leaked another skill: %+v", ev)
		}
	}
	all, err := store.ListEvidence("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected all three records, got %d", len(all))
	}
}

// M1-32: clearing evidence needs explicit confirmation and drops only the
// records asked for.
func TestClearEvidenceRequiresConfirmationAndRewritesLog(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, t.TempDir())
	for _, skill := range []string{"demo", "other"} {
		if _, err := store.AppendEvidence(Evidence{Kind: "tool-failure", SessionID: "s1", Skill: skill, Summary: skill + " failed"}); err != nil {
			t.Fatal(err)
		}
	}

	before, err := os.ReadFile(filepath.Join(dataDir, evidenceFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClearEvidence("demo", false); err == nil {
		t.Fatal("clearing without confirmation must be refused")
	}
	after, err := os.ReadFile(filepath.Join(dataDir, evidenceFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a refused clear must not touch the log")
	}

	removed, err := store.ClearEvidence("demo", true)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("expected one removed record, got %d", removed)
	}
	remaining, err := store.ListEvidence("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Skill != "other" {
		t.Fatalf("clear should keep the other skill: %+v", remaining)
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, evidenceFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "demo failed") {
		t.Fatalf("cleared record is still in the log: %s", raw)
	}
}

// M1-33: turning collection off stops new records; turning it back on resumes.
func TestCollectionToggleStopsNewEvidence(t *testing.T) {
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, workspace)
	runner := agent.NewHookRunner(nil, workspace)
	runner.AddHandlers(Hooks(Context{DataDir: dataDir, WorkspaceRoot: workspace})...)

	if !store.CollectionEnabled() {
		t.Fatal("collection should be on by default")
	}
	if err := store.SetCollectionEnabled(false); err != nil {
		t.Fatal(err)
	}
	_ = runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "以后 $demo 要检查测试"))

	if store.CollectionEnabled() {
		t.Fatal("collection should stay off after being disabled")
	}
	evs, err := store.ListEvidence("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 {
		t.Fatalf("disabled collection must not record evidence: %+v", evs)
	}
	if _, err := os.Stat(filepath.Join(dataDir, evidenceFileName)); !os.IsNotExist(err) {
		t.Fatalf("disabled collection should not create the log, stat err=%v", err)
	}

	// A fresh store sees the same choice: it is on disk, not in memory.
	reopened := NewStore(dataDir, workspace)
	if reopened.CollectionEnabled() {
		t.Fatal("the collection choice should survive a restart")
	}
	if err := reopened.SetCollectionEnabled(true); err != nil {
		t.Fatal(err)
	}
	_ = runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "以后 $demo 要检查测试"))
	evs, err = reopened.ListEvidence("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("resumed collection should record again, got %+v", evs)
	}
}

// M1-34: evidence stays in the private data dir and there is no upload path.
func TestEvidenceStaysPrivateAndUnuploadable(t *testing.T) {
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, workspace)

	if _, err := store.AppendEvidence(Evidence{Kind: "tool-failure", SessionID: "s1", Skill: "demo", Summary: "boom"}); err != nil {
		t.Fatal(err)
	}
	logPath := store.evidencePath()
	if !strings.HasPrefix(logPath, dataDir+string(filepath.Separator)) {
		t.Fatalf("evidence log is not in the private data dir: %s", logPath)
	}
	if strings.HasPrefix(logPath, workspace+string(filepath.Separator)) {
		t.Fatalf("evidence log must not live in the user's project: %s", logPath)
	}

	// The package has no network import, so nothing here can send a record out.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(ent.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{`"net/` + `http"`, `"net/` + `smtp"`, `"net/` + `rpc"`} {
			if strings.Contains(string(b), banned) {
				t.Fatalf("%s imports %s, which would give evidence an upload path", ent.Name(), banned)
			}
		}
	}
}

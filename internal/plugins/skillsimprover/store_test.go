package skillsimprover

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/securefs"
)

func TestStoreEvidenceAndProposalApply(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)

	ev, err := store.AppendEvidence(Evidence{Kind: "user-feedback", SessionID: "s1", Skill: "demo", Prompt: "下次 $demo 要更具体"})
	if err != nil {
		t.Fatalf("append evidence: %v", err)
	}
	if ev.ID == "" {
		t.Fatal("expected evidence id")
	}
	evs, err := store.ListEvidence("demo", 10)
	if err != nil || len(evs) != 1 {
		t.Fatalf("list evidence len=%d err=%v", len(evs), err)
	}

	orig, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	proposed := skillMarkdown("demo", "new body")
	p, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "make demo more specific",
		ProposedSkillMD: proposed,
		EvidenceIDs:     []string{ev.ID},
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	applied, err := store.ApplyProposal(p.ID)
	if err != nil {
		t.Fatalf("apply proposal: %v", err)
	}
	if applied.AppliedAt.IsZero() {
		t.Fatal("expected applied timestamp")
	}
	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "new body") {
		t.Fatalf("proposal not applied:\n%s", string(got))
	}
}

func TestApplyProposalRejectsChangedSkill(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	orig, _ := os.ReadFile(skillPath)
	p, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte(skillMarkdown("demo", "external change")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyProposal(p.ID); err == nil || !strings.Contains(err.Error(), "sha mismatch") {
		t.Fatalf("expected sha mismatch, got %v", err)
	}
}

func TestSaveProposalIgnoresCallerSuppliedID(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, workspace)
	orig, _ := os.ReadFile(skillPath)
	p, err := store.SaveProposal(Proposal{
		ID:              "../escape",
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	if strings.Contains(p.ID, "..") || strings.ContainsAny(p.ID, `/\`) || p.ID == "../escape" {
		t.Fatalf("caller supplied id was not regenerated: %q", p.ID)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "escape.json")); !os.IsNotExist(err) {
		t.Fatalf("proposal escaped proposals dir, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "proposals", p.ID+".json")); err != nil {
		t.Fatalf("expected sanitized proposal file: %v", err)
	}
}

func TestSaveProposalPreservesUppercaseSkillName(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "MySkill", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	orig, _ := os.ReadFile(skillPath)
	p, err := store.SaveProposal(Proposal{
		Skill:           "MySkill",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("MySkill", "new body"),
	})
	if err != nil {
		t.Fatalf("save uppercase proposal: %v", err)
	}
	if p.Skill != "MySkill" {
		t.Fatalf("skill name lowercased: %q", p.Skill)
	}
	if _, err := store.ApplyProposal(p.ID); err != nil {
		t.Fatalf("apply uppercase proposal: %v", err)
	}
}

func TestSaveProposalRequiresOriginalSHA(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	_, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err == nil || !strings.Contains(err.Error(), "original_sha256 is required") {
		t.Fatalf("expected missing original_sha256 error, got %v", err)
	}
}

func TestApplyProposalAcceptsSymlinkEquivalentPath(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	linkRoot := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(workspace, linkRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	linkSkillPath := filepath.Join(linkRoot, ".whale", "skills", "demo", "SKILL.md")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	orig, _ := os.ReadFile(skillPath)
	p, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   linkSkillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err != nil {
		t.Fatalf("save proposal with symlink-equivalent path: %v", err)
	}
	if _, err := store.ApplyProposal(p.ID); err != nil {
		t.Fatalf("apply proposal with symlink-equivalent path: %v", err)
	}
}

func TestHooksCaptureFeedbackAndFailuresOnly(t *testing.T) {
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	handlers := Hooks(Context{DataDir: dataDir, WorkspaceRoot: workspace})
	runner := agent.NewHookRunner(nil, workspace)
	runner.AddHandlers(handlers...)

	pass := runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "普通请求"))
	if len(pass.Outcomes) != 1 || pass.Outcomes[0].Decision != agent.HookDecisionPass {
		t.Fatalf("unexpected pass hook report: %+v", pass)
	}
	store := NewStore(dataDir, workspace)
	evs, err := store.ListEvidence("", 10)
	if err != nil || len(evs) != 0 {
		t.Fatalf("ordinary prompt should not record evidence: len=%d err=%v", len(evs), err)
	}

	_ = runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "what should I do next?"))
	evs, err = store.ListEvidence("", 10)
	if err != nil || len(evs) != 0 {
		t.Fatalf("broad feedback keyword without skill should not record evidence: len=%d err=%v", len(evs), err)
	}

	_ = runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "$demo run the tests"))
	evs, err = store.ListEvidence("", 10)
	if err != nil || len(evs) != 0 {
		t.Fatalf("ordinary skill invocation should not record evidence: len=%d err=%v", len(evs), err)
	}

	_ = runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "以后 $demo 要检查测试"))
	evs, err = store.ListEvidence("demo", 10)
	if err != nil || len(evs) != 1 || evs[0].Kind != "user-feedback" {
		t.Fatalf("expected feedback evidence, got %+v err=%v", evs, err)
	}

	okRes := core.NewToolResultSuccess(core.ToolCall{ID: "c1", Name: "shell_run"}, map[string]any{"status": "ok"}, nil)
	_ = runner.RunHook(t.Context(), agent.NewPostToolUsePayload("s1", core.ToolCall{Name: "shell_run", Input: `{"command":"go test"}`}, nil, okRes))
	evs, _ = store.ListEvidence("", 10)
	if len(evs) != 1 {
		t.Fatalf("successful tool should not add evidence: %+v", evs)
	}

	failRes := core.NewToolResultError(core.ToolCall{ID: "c2", Name: "shell_run"}, "exec_failed", "tests failed", nil)
	_ = runner.RunHook(t.Context(), agent.NewPostToolUsePayload("s1", core.ToolCall{Name: "shell_run", Input: `{"command":"go test"}`}, map[string]any{"command": "go test"}, failRes))
	evs, _ = store.ListEvidence("", 10)
	if len(evs) != 2 || evs[0].Kind == "" {
		t.Fatalf("expected tool failure evidence: %+v", evs)
	}
}

func TestStopHookSummarizesOnlyUnsummarizedEvidence(t *testing.T) {
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	handlers := Hooks(Context{DataDir: dataDir, WorkspaceRoot: workspace})
	runner := agent.NewHookRunner(nil, workspace)
	runner.AddHandlers(handlers...)
	store := NewStore(dataDir, workspace)

	_ = runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "以后 $demo 要检查测试"))
	_ = runner.RunHook(t.Context(), agent.NewStopPayload("s1", workspace, "first answer", 1))
	evs, err := store.ListEvidence("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if countEvidenceKind(evs, "turn-summary") != 1 {
		t.Fatalf("expected one turn summary after evidence, got %+v", evs)
	}
	summaries, err := store.ListEvidence("demo", 10)
	if err != nil {
		t.Fatal(err)
	}
	var linkedSummary Evidence
	for _, ev := range summaries {
		if ev.Kind == "turn-summary" {
			linkedSummary = ev
			break
		}
	}
	if linkedSummary.Skill != "demo" || !strings.Contains(linkedSummary.Prompt, "$demo") || linkedSummary.Metadata["source_evidence_id"] == "" {
		t.Fatalf("turn summary should preserve skill linkage, got %+v", linkedSummary)
	}

	_ = runner.RunHook(t.Context(), agent.NewStopPayload("s1", workspace, "unrelated later answer", 2))
	evs, err = store.ListEvidence("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if countEvidenceKind(evs, "turn-summary") != 1 {
		t.Fatalf("unrelated later stop should not append summary, got %+v", evs)
	}

	_ = runner.RunHook(t.Context(), agent.NewUserPromptSubmitPayload("s1", workspace, "下次 $demo 要更短"))
	_ = runner.RunHook(t.Context(), agent.NewStopPayload("s1", workspace, "second answer", 3))
	evs, err = store.ListEvidence("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if countEvidenceKind(evs, "turn-summary") != 2 {
		t.Fatalf("new evidence should allow a new summary, got %+v", evs)
	}
}

func writeTestSkill(t *testing.T, workspace, name, body string) string {
	t.Helper()
	return writeSkillAt(t, filepath.Join(workspace, ".whale", "skills"), name, body)
}

func writeSkillAt(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(skillMarkdown(name, body)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func skillMarkdown(name, body string) string {
	return "---\nname: " + name + "\ndescription: Demo skill\n---\n\n# Demo\n\n" + body + "\n"
}

func countEvidenceKind(evs []Evidence, kind string) int {
	count := 0
	for _, ev := range evs {
		if ev.Kind == kind {
			count++
		}
	}
	return count
}

func TestWriteTargetRejectsPathsOutsideProjectSkillRoots(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	homeSkill := writeSkillAt(t, filepath.Join(home, ".whale", "skills"), "demo", "old body")
	looseDir := filepath.Join(workspace, "elsewhere")
	if err := os.MkdirAll(looseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	looseSkill := writeSkillAt(t, looseDir, "demo", "old body")
	untracked := filepath.Join(workspace, ".whale", "skills", "not-a-skill", "README.md")
	if err := os.MkdirAll(filepath.Dir(untracked), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(untracked, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	save := func(target, name string) error {
		t.Helper()
		orig, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.SaveProposal(Proposal{
			Skill:           name,
			SkillFilePath:   target,
			OriginalSHA256:  sha256Hex(orig),
			Summary:         "change",
			ProposedSkillMD: skillMarkdown(name, "new body"),
		})
		return err
	}

	for _, target := range []string{homeSkill, looseSkill} {
		err := save(target, "demo")
		var rejected *RejectError
		if !errors.As(err, &rejected) || rejected.Code != RejectTargetOutsideProjectSkillRoots {
			t.Fatalf("target %s: want code %s, got %v", target, RejectTargetOutsideProjectSkillRoots, err)
		}
	}

	// Inside a project skill root but not the discovered skill: a plain error, not a rejection.
	if err := save(untracked, "demo"); err == nil || errors.As(err, new(*RejectError)) {
		t.Fatalf("path inside a root should fail discovery, not the root guard: %v", err)
	}

	// The other allowed project root still accepts writes.
	agentsSkill := writeSkillAt(t, filepath.Join(workspace, ".agents", "skills"), "demo2", "old body")
	if err := save(agentsSkill, "demo2"); err != nil {
		t.Fatalf("write target under .agents/skills should be accepted: %v", err)
	}
}
func TestWriteTargetRejectsPluginSkillPath(t *testing.T) {
	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, workspace)

	_, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   "plugin://skills-improver/SKILL.md",
		OriginalSHA256:  sha256Hex([]byte("old body")),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err == nil || !strings.Contains(err.Error(), "plugin") {
		t.Fatalf("save: want plugin target rejected, got %v", err)
	}

	// A hand-written proposal on disk must not reach the plugin path either.
	dir := filepath.Join(dataDir, "proposals")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Proposal{
		ID:              "sp-plugin",
		Skill:           "demo",
		SkillFilePath:   "plugin://skills-improver/SKILL.md",
		OriginalSHA256:  sha256Hex([]byte("old body")),
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sp-plugin.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyProposal("sp-plugin"); err == nil || !strings.Contains(err.Error(), "plugin") {
		t.Fatalf("apply: want plugin target rejected, got %v", err)
	}
}
func rejectCode(err error) string {
	var rejected *RejectError
	if errors.As(err, &rejected) {
		return rejected.Code
	}
	return ""
}

func TestWriteTargetRejectsSymlinksBelowProjectRoot(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	realSkill := writeTestSkill(t, workspace, "demo", "old body")
	orig, err := os.ReadFile(realSkill)
	if err != nil {
		t.Fatal(err)
	}

	linkedDir := filepath.Join(workspace, ".agents", "skills", "linked")
	if err := os.MkdirAll(filepath.Dir(linkedDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(realSkill), linkedDir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	linkedFileDir := filepath.Join(workspace, ".whale", "skills", "demo-link")
	if err := os.MkdirAll(linkedFileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realSkill, filepath.Join(linkedFileDir, "SKILL.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	targets := map[string]string{
		filepath.Join(linkedDir, "SKILL.md"):     "parent directory",
		filepath.Join(linkedFileDir, "SKILL.md"): "target file",
	}
	for target, kind := range targets {
		_, err := store.SaveProposal(Proposal{
			Skill:           "demo",
			SkillFilePath:   target,
			OriginalSHA256:  sha256Hex(orig),
			Summary:         "change",
			ProposedSkillMD: skillMarkdown("demo", "new body"),
		})
		if code := rejectCode(err); code != RejectTargetHasSymlink {
			t.Fatalf("%s symlink at %s: want code %s, got %v", kind, target, RejectTargetHasSymlink, err)
		}
	}

	after, err := os.ReadFile(realSkill)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(orig) {
		t.Fatalf("rejected symlink targets must not touch the real skill:\n%s", after)
	}
}

func TestWriteTargetRejectsSamePrefixSiblingDirectory(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "proj")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	evilSkill := writeSkillAt(t, filepath.Join(base, "proj-evil", ".whale", "skills"), "demo", "old body")
	orig, err := os.ReadFile(evilSkill)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	_, err = store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   evilSkill,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if code := rejectCode(err); code != RejectTargetOutsideProjectSkillRoots {
		t.Fatalf("same-prefix sibling %s must be outside the roots, got %v", evilSkill, err)
	}
}

func TestRequireRegularFileRejectsNonRegularTarget(t *testing.T) {
	dir := t.TempDir()
	if err := requireRegularFile(writeSkillAt(t, dir, "demo", "body")); err != nil {
		t.Fatalf("regular skill file should pass: %v", err)
	}
	if code := rejectCode(requireRegularFile(dir)); code != RejectTargetNotRegularFile {
		t.Fatalf("directory target: want code %s, got %v", RejectTargetNotRegularFile, requireRegularFile(dir))
	}
	if err := requireRegularFile(filepath.Join(dir, "missing", "SKILL.md")); err == nil {
		t.Fatal("missing target must not pass the regular file check")
	}
}
func TestWriteFileAtomicSyncsBeforeRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}

	var order []string
	origSync, origRename := atomicWriteSync, atomicWriteRename
	t.Cleanup(func() { atomicWriteSync, atomicWriteRename = origSync, origRename })
	atomicWriteSync = func(f *os.File) error {
		order = append(order, "sync")
		return f.Sync()
	}
	atomicWriteRename = func(oldpath, newpath string) error {
		order = append(order, "rename")
		return os.Rename(oldpath, newpath)
	}

	if err := writeFileAtomic(path, []byte("new\n")); err != nil {
		t.Fatalf("atomic write: %v", err)
	}
	if len(order) != 2 || order[0] != "sync" || order[1] != "rename" {
		t.Fatalf("want sync before rename, got %v", order)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("want replaced content, got %q", string(got))
	}
}

func TestWriteFileAtomicPreservesTargetModeAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new\n")); err != nil {
		t.Fatalf("atomic write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("want mode 0640 preserved, got %v", fi.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "SKILL.md" {
		t.Fatalf("temp file left behind: %v", entries)
	}

	failed := filepath.Join(dir, "other.md")
	if err := os.WriteFile(failed, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	origRename := atomicWriteRename
	t.Cleanup(func() { atomicWriteRename = origRename })
	atomicWriteRename = func(oldpath, newpath string) error { return errors.New("injected rename failure") }
	if err := writeFileAtomic(failed, []byte("new\n")); err == nil {
		t.Fatal("expected injected rename failure")
	}
	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind after failure: %s", e.Name())
		}
	}
	got, err := os.ReadFile(failed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("failed rename must leave target untouched, got %q", string(got))
	}
}
func TestApplyProposalKeepsOriginalBackupBeforeReplace(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	orig, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "second body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	applied, err := store.ApplyProposal(p.ID)
	if err != nil {
		t.Fatalf("apply proposal: %v", err)
	}
	if applied.OriginalBackupPath == "" {
		t.Fatal("expected the original to be backed up before the replace")
	}
	backup, err := os.ReadFile(applied.OriginalBackupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != string(orig) {
		t.Fatalf("backup must hold the original bytes, got %q", string(backup))
	}
	reloaded, err := store.ReadProposal(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.OriginalBackupPath != applied.OriginalBackupPath {
		t.Fatalf("backup path must persist on the proposal, got %q", reloaded.OriginalBackupPath)
	}

	// A failure while replacing the target still leaves a complete backup.
	current, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	failing, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(current),
		Summary:         "change again",
		ProposedSkillMD: skillMarkdown("demo", "third body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	origRename := atomicWriteRename
	t.Cleanup(func() { atomicWriteRename = origRename })
	atomicWriteRename = func(string, string) error { return errors.New("injected replace failure") }
	if _, err := store.ApplyProposal(failing.ID); err == nil {
		t.Fatal("expected the injected replace failure")
	}
	atomicWriteRename = origRename
	after, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(current) {
		t.Fatalf("target must be untouched when the replace fails, got %q", string(after))
	}
	kept, err := os.ReadFile(filepath.Join(store.backupsDir(), "sha256-"+sha256Hex(current)+".md"))
	if err != nil {
		t.Fatalf("backup written before the failed replace must survive: %v", err)
	}
	if string(kept) != string(current) {
		t.Fatal("backup written before the failed replace must be complete")
	}

	// If the backup cannot be written at all, apply aborts before the target.
	blockedStore := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	blocked, err := blockedStore.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(current),
		Summary:         "change with blocked backups",
		ProposedSkillMD: skillMarkdown("demo", "fourth body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	if err := os.WriteFile(blockedStore.backupsDir(), []byte("block backups"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := blockedStore.ApplyProposal(blocked.ID); err == nil {
		t.Fatal("apply must abort when the backup cannot be written")
	}
	untouched, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(untouched) != string(current) {
		t.Fatalf("target must be untouched when the backup fails, got %q", string(untouched))
	}
}
func TestApplyProposalAdvancesStateAndKeepsAppliedLast(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	orig, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	if p.State != StateProposed {
		t.Fatalf("saved proposal state = %q, want %q", p.State, StateProposed)
	}
	onDisk, err := store.ReadProposal(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.State != StateProposed {
		t.Fatalf("on-disk state = %q, want %q", onDisk.State, StateProposed)
	}

	// While the target is being replaced, the state on disk must already read
	// applying, so a crash there is identifiable rather than looking applied.
	var duringReplace string
	origRename := atomicWriteRename
	t.Cleanup(func() { atomicWriteRename = origRename })
	atomicWriteRename = func(oldpath, newpath string) error {
		if during, err := store.ReadProposal(p.ID); err == nil {
			duringReplace = during.State
		}
		return origRename(oldpath, newpath)
	}
	applied, err := store.ApplyProposal(p.ID)
	if err != nil {
		t.Fatalf("apply proposal: %v", err)
	}
	if duringReplace != StateApplying {
		t.Fatalf("state during replace = %q, want %q", duringReplace, StateApplying)
	}
	if applied.State != StateApplied || applied.AppliedAt.IsZero() {
		t.Fatalf("applied proposal state = %q at zero=%v", applied.State, applied.AppliedAt.IsZero())
	}
	final, err := store.ReadProposal(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != StateApplied {
		t.Fatalf("final on-disk state = %q, want %q", final.State, StateApplied)
	}
	if _, err := store.ApplyProposal(p.ID); err == nil {
		t.Fatal("an applied proposal must not apply twice")
	}
}

func TestProposalStateDefaultsForOlderFiles(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	store := NewStore(dataDir, workspace)
	if err := securefs.MkdirPrivate(store.proposalsDir()); err != nil {
		t.Fatal(err)
	}
	// A proposal written before the state machine: applied_at decides.
	appliedJSON := `{"id":"sp-legacy","skill":"demo","skill_file_path":` + strconv.Quote(skillPath) +
		`,"summary":"legacy","proposed_skill_md":"x","applied_at":"2026-01-02T03:04:05Z"}`
	if err := securefs.WritePrivateFile(filepath.Join(store.proposalsDir(), "sp-legacy.json"), []byte(appliedJSON)); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.ReadProposal("sp-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got := legacy.effectiveState(); got != StateApplied {
		t.Fatalf("legacy applied proposal state = %q, want %q", got, StateApplied)
	}
	if _, err := store.ApplyProposal("sp-legacy"); err == nil {
		t.Fatal("a legacy applied proposal must not apply again")
	}

	proposedJSON := `{"id":"sp-legacy-open","skill":"demo","skill_file_path":` + strconv.Quote(skillPath) +
		`,"original_sha256":"` + sha256Hex([]byte("old body")) + `","summary":"legacy open","proposed_skill_md":"x"}`
	if err := securefs.WritePrivateFile(filepath.Join(store.proposalsDir(), "sp-legacy-open.json"), []byte(proposedJSON)); err != nil {
		t.Fatal(err)
	}
	open, err := store.ReadProposal("sp-legacy-open")
	if err != nil {
		t.Fatal(err)
	}
	if got := open.effectiveState(); got != StateProposed {
		t.Fatalf("legacy open proposal state = %q, want %q", got, StateProposed)
	}
}

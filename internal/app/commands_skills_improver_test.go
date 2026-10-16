package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/plugins"
	"github.com/usewhale/whale/internal/plugins/skillsimprover"
)

// skillProposalFixture enables the plugin the way a user does and seeds one
// proposal against a project skill.
func skillProposalFixture(t *testing.T, enabled bool) (*App, *skillsimprover.Store, string, skillsimprover.Proposal) {
	t.Helper()
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	workspace := t.TempDir()
	t.Chdir(workspace)
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	if enabled {
		cfg.Plugins = plugins.ConfigMap{"skills-improver": {Enabled: boolPtr(true)}}
		// Only the user-level config grants the capability (M1-40/M1-42).
		writeGlobalConfig(t, cfg.DataDir, "[plugins.skills-improver]\nenabled = true\n")
	}
	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	dir := filepath.Join(workspace, ".whale", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "SKILL.md")
	original := demoSkillMarkdown("original body")
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	store := skillsimprover.StoreFor(skillsimprover.Context{
		DataDir:       filepath.Join(cfg.DataDir, "plugins", skillsimprover.PluginID, "data"),
		WorkspaceRoot: workspace,
	})
	p, err := store.SaveProposal(skillsimprover.Proposal{
		Skill:           "demo",
		SkillFilePath:   target,
		OriginalSHA256:  sha256Of(t, original),
		ProposedSkillMD: demoSkillMarkdown("proposed body"),
		Summary:         "make the demo skill specific",
	})
	if err != nil {
		t.Fatalf("SaveProposal: %v", err)
	}
	return app, store, target, p
}

func demoSkillMarkdown(body string) string {
	return "---\nname: demo\ndescription: Demo skill\n---\n\n# Demo\n\n" + body + "\n"
}

func sha256Of(t *testing.T, content string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestSkillProposalApplyAsksBeforeWriting(t *testing.T) {
	app, _, target, p := skillProposalFixture(t, true)

	res, err := app.ExecuteLocalCommand("/skills-improver apply " + p.ID)
	if err != nil {
		t.Fatalf("apply line: %v", err)
	}
	if !res.Handled || res.LocalResult == nil || res.LocalResult.Kind != skillProposalResultKind {
		t.Fatalf("apply line should open the confirmation prompt: %+v", res)
	}
	if got := readFile(t, target); got != demoSkillMarkdown("original body") {
		t.Fatal("asking for confirmation must not write the skill")
	}

	var approval *SkillProposalApproval
	for _, action := range res.LocalResult.Actions {
		if action.SkillProposal != nil {
			approval = action.SkillProposal
		}
	}
	if approval == nil {
		t.Fatalf("confirmation prompt has no approve action: %+v", res.LocalResult.Actions)
	}
	if approval.ID != p.ID || approval.Action != skillsimprover.ActionApply {
		t.Fatalf("approval does not name the apply: %+v", approval)
	}
	if approval.Target != target || approval.SessionID == "" {
		t.Fatalf("approval must bind session and target: %+v", approval)
	}
	if approval.CurrentSHA != sha256Of(t, demoSkillMarkdown("original body")) {
		t.Fatalf("approval current sha = %q", approval.CurrentSHA)
	}
	if approval.ResultSHA != sha256Of(t, strings.TrimSpace(demoSkillMarkdown("proposed body"))+"\n") {
		t.Fatalf("approval result sha = %q", approval.ResultSHA)
	}

	// An approval that no longer matches the proposal writes nothing (M1-52).
	tampered := *approval
	tampered.ResultSHA = strings.Repeat("b", 64)
	if _, err := app.ApplySkillProposal(tampered); err == nil {
		t.Fatal("a drifted approval must be refused")
	}
	if got := readFile(t, target); got != demoSkillMarkdown("original body") {
		t.Fatal("a refused approval must leave the skill alone")
	}

	// An approval without a session is not an approval (M1-53).
	unbound := *approval
	unbound.SessionID = ""
	if _, err := app.ApplySkillProposal(unbound); err == nil {
		t.Fatal("an approval without a session must be refused")
	}

	msg, err := app.ApplySkillProposal(*approval)
	if err != nil {
		t.Fatalf("confirmed apply: %v", err)
	}
	if !strings.Contains(msg, p.ID) {
		t.Fatalf("apply message should name the proposal: %q", msg)
	}
	if got, want := readFile(t, target), strings.TrimSpace(demoSkillMarkdown("proposed body"))+"\n"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func TestSkillProposalRestoreAsksBeforeWriting(t *testing.T) {
	app, _, target, p := skillProposalFixture(t, true)

	res, err := app.ExecuteLocalCommand("/skills-improver apply " + p.ID)
	if err != nil {
		t.Fatalf("apply line: %v", err)
	}
	var applyApproval *SkillProposalApproval
	for _, action := range res.LocalResult.Actions {
		if action.SkillProposal != nil {
			applyApproval = action.SkillProposal
		}
	}
	if _, err := app.ApplySkillProposal(*applyApproval); err != nil {
		t.Fatalf("confirmed apply: %v", err)
	}

	restore, err := app.ExecuteLocalCommand("/skills-improver restore " + p.ID)
	if err != nil {
		t.Fatalf("restore line: %v", err)
	}
	var restoreApproval *SkillProposalApproval
	for _, action := range restore.LocalResult.Actions {
		if action.SkillProposal != nil {
			restoreApproval = action.SkillProposal
		}
	}
	if restoreApproval == nil || restoreApproval.Action != skillsimprover.ActionRestore {
		t.Fatalf("restore prompt has no restore approval: %+v", restore.LocalResult.Actions)
	}
	if restoreApproval.ResultSHA != sha256Of(t, demoSkillMarkdown("original body")) {
		t.Fatalf("restore result sha = %q", restoreApproval.ResultSHA)
	}
	// The apply approval must not approve the restore.
	if _, err := app.ApplySkillProposal(*applyApproval); err == nil {
		t.Fatal("an apply approval must not approve a restore")
	}
	if _, err := app.ApplySkillProposal(*restoreApproval); err != nil {
		t.Fatalf("confirmed restore: %v", err)
	}
	if got := readFile(t, target); got != demoSkillMarkdown("original body") {
		t.Fatalf("restore did not put the original back: %q", got)
	}
}

func TestSkillProposalCommandsStayOffWhilePluginIsDisabled(t *testing.T) {
	app, _, target, p := skillProposalFixture(t, false)

	for _, line := range []string{"/skills-improver apply " + p.ID, "/skills-improver restore " + p.ID} {
		res, err := app.ExecuteLocalCommand(line)
		if err != nil || res.Handled {
			t.Fatalf("%s should stay hidden: handled=%v err=%v", line, res.Handled, err)
		}
	}
	if _, err := app.ApplySkillProposal(SkillProposalApproval{
		ID:         p.ID,
		Action:     skillsimprover.ActionApply,
		SessionID:  "s1",
		Target:     target,
		CurrentSHA: p.OriginalSHA256,
		ResultSHA:  sha256Of(t, strings.TrimSpace(demoSkillMarkdown("proposed body"))+"\n"),
	}); err == nil {
		t.Fatal("a disabled plugin must refuse the write")
	}
	if got := readFile(t, target); got != demoSkillMarkdown("original body") {
		t.Fatal("a disabled plugin must not write the skill")
	}
}

// TestSkillProposalApplyAcceptsNoSkipSuffix pins M1-55: no argument turns the
// apply line into a one-step write, so there is no "skip the confirmation"
// entry point.
func TestSkillProposalApplyAcceptsNoSkipSuffix(t *testing.T) {
	app, _, target, p := skillProposalFixture(t, true)

	for _, suffix := range []string{" --yes", " -y", " force", " confirm", " skip"} {
		res, err := app.ExecuteLocalCommand("/skills-improver apply " + p.ID + suffix)
		if got := readFile(t, target); got != demoSkillMarkdown("original body") {
			t.Fatalf("apply%s wrote the skill without confirmation", suffix)
		}
		if err == nil && res.LocalResult != nil {
			for _, action := range res.LocalResult.Actions {
				if action.SkillProposal != nil {
					t.Fatalf("apply%s must not hand out an approval", suffix)
				}
			}
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

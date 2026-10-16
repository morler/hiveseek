package skillsimprover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/skills"
)

// proposalForTarget saves one proposal against a fresh project skill and
// returns the store, the workspace and the proposal.
func seededProposal(t *testing.T, proposedBody string) (*Store, string, Proposal) {
	t.Helper()
	dataDir := t.TempDir()
	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".whale", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "SKILL.md")
	original := skillMarkdown("demo", "original body")
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dataDir, workspace)
	p, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   target,
		OriginalSHA256:  sha256Hex([]byte(original)),
		ProposedSkillMD: skillMarkdown("demo", proposedBody),
		Summary:         "make the demo skill specific",
	})
	if err != nil {
		t.Fatalf("SaveProposal: %v", err)
	}
	return store, workspace, p
}

func TestParseDecisionMatchesMutatingFormsOnly(t *testing.T) {
	cases := []struct {
		line   string
		id     string
		action string
		ok     bool
	}{
		{"/skills-improver apply sp-1", "sp-1", ActionApply, true},
		{"/skills-improver restore  sp-2", "sp-2", ActionRestore, true},
		{"/skills-improver status", "", "", false},
		{"/skills-improver proposals", "", "", false},
		{"/skills-improver show sp-1", "", "", false},
		{"/skills-improver apply", "", "", false},
		{"/skills-improver apply sp-1 extra", "", "", false},
		{"skills-improver apply sp-1", "", "", false},
		{"/memory apply sp-1", "", "", false},
	}
	for _, tc := range cases {
		id, action, ok := ParseDecision(tc.line)
		if ok != tc.ok || id != tc.id || action != tc.action {
			t.Errorf("ParseDecision(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.line, id, action, ok, tc.id, tc.action, tc.ok)
		}
	}
}

func TestDecisionDescribesAnApplyWithoutWriting(t *testing.T) {
	store, _, p := seededProposal(t, "proposed body")

	decision, err := store.PendingDecision(p.ID, ActionApply)
	if err != nil {
		t.Fatalf("PendingDecision: %v", err)
	}
	if decision.Skill != "demo" || decision.Target != p.SkillFilePath {
		t.Fatalf("decision does not describe the proposal: %+v", decision)
	}
	if decision.CurrentSHA != p.OriginalSHA256 {
		t.Fatalf("current sha = %q, want the original sha", decision.CurrentSHA)
	}
	if decision.ResultSHA != sha256Hex(proposedSkillBytes(p)) {
		t.Fatalf("result sha = %q, want the proposed content hash", decision.ResultSHA)
	}
	text := decision.DecisionText()
	for _, want := range []string{p.ID, "demo", p.SkillFilePath, decision.CurrentSHA, decision.ResultSHA} {
		if !strings.Contains(text, want) {
			t.Errorf("confirmation text is missing %q:\n%s", want, text)
		}
	}
	// Nothing is written before the user confirms.
	got, err := os.ReadFile(p.SkillFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256Hex(got) != p.OriginalSHA256 {
		t.Fatal("PendingDecision must not touch the target skill")
	}
}

func TestDecideNeedsAnApprovalThatMatchesTheProposal(t *testing.T) {
	store, _, p := seededProposal(t, "proposed body")
	decision, err := store.PendingDecision(p.ID, ActionApply)
	if err != nil {
		t.Fatal(err)
	}
	matching := Approval{
		SessionID:  "s1",
		Target:     decision.Target,
		CurrentSHA: decision.CurrentSHA,
		ResultSHA:  decision.ResultSHA,
	}

	cases := []struct {
		name string
		ap   Approval
	}{
		{"no session", Approval{Target: matching.Target, CurrentSHA: matching.CurrentSHA, ResultSHA: matching.ResultSHA}},
		{"other target", Approval{SessionID: "s1", Target: "/tmp/other/SKILL.md", CurrentSHA: matching.CurrentSHA, ResultSHA: matching.ResultSHA}},
		{"drifted current sha", Approval{SessionID: "s1", Target: matching.Target, CurrentSHA: strings.Repeat("a", 64), ResultSHA: matching.ResultSHA}},
		{"drifted result sha", Approval{SessionID: "s1", Target: matching.Target, CurrentSHA: matching.CurrentSHA, ResultSHA: strings.Repeat("b", 64)}},
	}
	for _, tc := range cases {
		if _, err := store.Decide(p.ID, ActionApply, tc.ap); err == nil {
			t.Errorf("%s: approval should be refused", tc.name)
		} else if rejectCode(err) != RejectApprovalInvalid {
			t.Errorf("%s: code = %q, want %q", tc.name, rejectCode(err), RejectApprovalInvalid)
		}
		got, err := os.ReadFile(p.SkillFilePath)
		if err != nil {
			t.Fatal(err)
		}
		if sha256Hex(got) != p.OriginalSHA256 {
			t.Fatalf("%s: refused approval must not touch the target", tc.name)
		}
	}

	applied, err := store.Decide(p.ID, ActionApply, matching)
	if err != nil {
		t.Fatalf("Decide with a matching approval: %v", err)
	}
	if applied.effectiveState() != StateApplied {
		t.Fatalf("state = %q, want applied", applied.effectiveState())
	}
	got, err := os.ReadFile(p.SkillFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256Hex(got) != decision.ResultSHA {
		t.Fatal("confirmed apply did not write the proposed content")
	}
}

func TestDecideRestoreRunsOnlyForAnAppliedProposal(t *testing.T) {
	store, _, p := seededProposal(t, "proposed body")
	decision, err := store.PendingDecision(p.ID, ActionApply)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Decide(p.ID, ActionRestore, Approval{SessionID: "s1"}); err == nil {
		t.Fatal("a proposed proposal cannot be restored")
	}

	if _, err := store.Decide(p.ID, ActionApply, Approval{
		SessionID:  "s1",
		Target:     decision.Target,
		CurrentSHA: decision.CurrentSHA,
		ResultSHA:  decision.ResultSHA,
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	restore, err := store.PendingDecision(p.ID, ActionRestore)
	if err != nil {
		t.Fatalf("PendingDecision(restore): %v", err)
	}
	if restore.CurrentSHA != decision.ResultSHA || restore.ResultSHA != decision.CurrentSHA {
		t.Fatalf("restore decision should describe the inverse of the apply: %+v", restore)
	}
	// An approval made for the apply must not approve the restore.
	if _, err := store.Decide(p.ID, ActionRestore, Approval{
		SessionID:  "s1",
		Target:     decision.Target,
		CurrentSHA: decision.CurrentSHA,
		ResultSHA:  decision.ResultSHA,
	}); err == nil || rejectCode(err) != RejectApprovalInvalid {
		t.Fatalf("apply approval should not approve a restore: %v", err)
	}

	if _, err := store.Decide(p.ID, ActionRestore, Approval{
		SessionID:  "s1",
		Target:     restore.Target,
		CurrentSHA: restore.CurrentSHA,
		ResultSHA:  restore.ResultSHA,
	}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := os.ReadFile(p.SkillFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256Hex(got) != p.OriginalSHA256 {
		t.Fatal("confirmed restore did not put the original content back")
	}
}

func TestHandleCommandRefusesABareDecision(t *testing.T) {
	store, _, p := seededProposal(t, "proposed body")

	out, mutated, err := HandleCommand(store, "/skills-improver apply "+p.ID)
	if err == nil {
		t.Fatal("a bare apply line must not run the write")
	}
	if !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("refusal should name the confirmation prompt: %v", err)
	}
	if mutated || strings.TrimSpace(out) != "" {
		t.Fatalf("refused decision changed state: mutated=%v out=%q", mutated, out)
	}
	got, readErr := os.ReadFile(p.SkillFilePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if sha256Hex(got) != p.OriginalSHA256 {
		t.Fatal("refused decision must not touch the target")
	}
	// The read-only forms still work.
	for _, line := range []string{"/skills-improver status", "/skills-improver proposals", "/skills-improver show " + p.ID} {
		if _, _, err := HandleCommand(store, line); err != nil {
			t.Errorf("%s: %v", line, err)
		}
	}
}

func TestConfirmableTargetIsAProjectSkillFile(t *testing.T) {
	_, workspace, p := seededProposal(t, "proposed body")
	roots := skills.DefaultRoots(workspace)
	inside := false
	for _, root := range roots {
		if strings.HasPrefix(p.SkillFilePath, root+string(filepath.Separator)) {
			inside = true
		}
	}
	if !inside {
		t.Fatalf("seeded target %s is not below a project skill root %v", p.SkillFilePath, roots)
	}
}

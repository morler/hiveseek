package skillsimprover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleCommandProposalsListsIDsAndStates(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)

	out, mutated, err := HandleCommand(store, "/skills-improver proposals")
	if err != nil || mutated {
		t.Fatalf("empty proposals: err=%v mutated=%v", err, mutated)
	}
	if !strings.Contains(out, "no proposals") {
		t.Fatalf("empty output should say so:\n%s", out)
	}

	orig, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	open, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "still open",
		ProposedSkillMD: skillMarkdown("demo", "new body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	applied, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "will be applied",
		ProposedSkillMD: skillMarkdown("demo", "applied body"),
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}
	if _, err := store.ApplyProposal(applied.ID); err != nil {
		t.Fatalf("apply proposal: %v", err)
	}

	out, mutated, err = HandleCommand(store, "/skills-improver proposals")
	if err != nil || mutated {
		t.Fatalf("proposals: err=%v mutated=%v", err, mutated)
	}
	for _, want := range []string{"2 proposal(s)", open.ID + "  " + StateProposed, applied.ID + "  " + StateApplied} {
		if !strings.Contains(out, want) {
			t.Fatalf("proposals output missing %q:\n%s", want, out)
		}
	}
}

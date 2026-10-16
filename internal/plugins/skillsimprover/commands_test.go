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

func TestHandleCommandShowPrintsProposalDiff(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)

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
		Risk:            "check wording",
		ProposedSkillMD: proposed,
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}

	// The original side comes from the target while the proposal is unapplied.
	out, mutated, err := HandleCommand(store, "/skills-improver show "+p.ID)
	if err != nil || mutated {
		t.Fatalf("show: err=%v mutated=%v", err, mutated)
	}
	for _, want := range []string{
		"proposal " + p.ID,
		"state: " + StateProposed,
		"skill: demo",
		"target: " + skillPath,
		"original sha256: " + sha256Hex(orig),
		"proposed sha256: " + sha256Hex([]byte(strings.TrimSpace(proposed)+"\n")),
		"risk: check wording",
		"--- original",
		"+++ proposed",
		"-old body",
		"+new body",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}

	// After the apply the original side comes from the private backup, so the
	// diff survives the replacement.
	if _, err := store.ApplyProposal(p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	out, _, err = HandleCommand(store, "/skills-improver show "+p.ID)
	if err != nil {
		t.Fatalf("show after apply: %v", err)
	}
	if !strings.Contains(out, "-old body") || !strings.Contains(out, "+new body") {
		t.Fatalf("diff missing after apply:\n%s", out)
	}

	if _, _, err := HandleCommand(store, "/skills-improver show"); err == nil {
		t.Fatal("show without an id should return the usage error")
	}
	if _, _, err := HandleCommand(store, "/skills-improver show sp-missing"); err == nil {
		t.Fatal("unknown proposal should be reported")
	}
}

func TestDiffLinesMarksContextRemovalsAndAdditions(t *testing.T) {
	got := strings.Join(diffLines("head\nold\ntail\n", "head\nnew\ntail\n"), "\n")
	want := " head\n-old\n+new\n tail"
	if got != want {
		t.Fatalf("diff mismatch:\n got %q\nwant %q", got, want)
	}
	if got := diffLines("", "a\n"); strings.Join(got, "|") != "+a" {
		t.Fatalf("empty original should be all additions, got %v", got)
	}
}

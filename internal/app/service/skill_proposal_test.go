package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/plugins"
	"github.com/usewhale/whale/internal/plugins/skillsimprover"
	"github.com/usewhale/whale/internal/runtime/protocol"
)

func TestSkillProposalIntentsMapToServiceKinds(t *testing.T) {
	for _, tt := range []struct {
		in   protocol.IntentKind
		want IntentKind
	}{
		{protocol.IntentSkillProposalApply, IntentSkillProposalApply},
		{protocol.IntentSkillProposalRestore, IntentSkillProposalRestore},
	} {
		if got := serviceIntentKind(tt.in); got != tt.want {
			t.Fatalf("serviceIntentKind(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSkillProposalApprovalMapsThroughProtocol(t *testing.T) {
	in := &protocol.SkillProposalApproval{
		ID:         "sp-1",
		Action:     skillsimprover.ActionApply,
		SessionID:  "s1",
		Target:     "/w/.whale/skills/demo/SKILL.md",
		CurrentSHA: "aaa",
		ResultSHA:  "bbb",
	}
	appApproval := appSkillProposalApproval(in)
	if appApproval == nil || appApproval.ID != in.ID || appApproval.Action != in.Action ||
		appApproval.SessionID != in.SessionID || appApproval.Target != in.Target ||
		appApproval.CurrentSHA != in.CurrentSHA || appApproval.ResultSHA != in.ResultSHA {
		t.Fatalf("unexpected app approval: %+v", appApproval)
	}
	back := protocolSkillProposalApproval(appApproval)
	if back == nil || *back != *in {
		t.Fatalf("round trip = %+v, want %+v", back, in)
	}
	if protocolSkillProposalApproval(nil) != nil || appSkillProposalApproval(nil) != nil {
		t.Fatal("a nil approval must stay nil")
	}
}

func TestRunSkillProposalDecisionReportsTheConfirmedWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	a, store, target := skillProposalServiceFixture(t)
	s := &Service{app: a, ctx: ctx, events: make(chan Event, 4)}

	// Without an approval there is nothing to execute.
	s.runSkillProposalDecision(nil)
	refused := <-s.events
	if refused.Kind != EventLocalSubmitResult || refused.Status != "error" ||
		!strings.Contains(refused.Text, "missing confirmation") {
		t.Fatalf("unexpected refusal event: %+v", refused)
	}
	original, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != string(original) {
		t.Fatalf("a refusal must not write, target = %q", got)
	}

	proposals, err := store.ListProposals()
	if err != nil || len(proposals) != 1 {
		t.Fatalf("want one proposal, got %v (%d)", err, len(proposals))
	}
	d, err := store.PendingDecision(proposals[0].ID, skillsimprover.ActionApply)
	if err != nil {
		t.Fatalf("PendingDecision: %v", err)
	}
	s.runSkillProposalDecision(&app.SkillProposalApproval{
		ID:         d.ProposalID,
		Action:     d.Action,
		SessionID:  a.SessionID(),
		Target:     d.Target,
		CurrentSHA: d.CurrentSHA,
		ResultSHA:  d.ResultSHA,
	})
	done := <-s.events
	if done.Kind != EventLocalSubmitResult || done.Status != "ok" ||
		!strings.Contains(done.Text, "applied skill demo") {
		t.Fatalf("unexpected result event: %+v", done)
	}
	want := "---\nname: demo\ndescription: Demo skill\n---\n\n# Demo\n\nproposed body\n"
	if got, _ := os.ReadFile(target); string(got) != want {
		t.Fatalf("target = %q, want the proposed body", got)
	}
}

func skillProposalServiceFixture(t *testing.T) (*app.App, *skillsimprover.Store, string) {
	t.Helper()
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	workspace := t.TempDir()
	t.Chdir(workspace)
	cfg := app.DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.Plugins = plugins.ConfigMap{"skills-improver": {Enabled: boolPtr(true)}}
	// Only the user-level config grants the capability (M1-40/M1-42).
	globalPath := app.GlobalConfigPath(cfg.DataDir)
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalPath, []byte("[plugins.skills-improver]\nenabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := app.New(t.Context(), cfg, app.StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	dir := filepath.Join(workspace, ".whale", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "SKILL.md")
	original := "---\nname: demo\ndescription: Demo skill\n---\n\n# Demo\n\noriginal body\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	store := skillsimprover.StoreFor(skillsimprover.Context{
		DataDir:       filepath.Join(cfg.DataDir, "plugins", skillsimprover.PluginID, "data"),
		WorkspaceRoot: workspace,
	})
	sum := sha256.Sum256([]byte(original))
	if _, err := store.SaveProposal(skillsimprover.Proposal{
		Skill:           "demo",
		SkillFilePath:   target,
		OriginalSHA256:  hex.EncodeToString(sum[:]),
		ProposedSkillMD: "---\nname: demo\ndescription: Demo skill\n---\n\n# Demo\n\nproposed body\n",
		Summary:         "make the demo skill specific",
	}); err != nil {
		t.Fatalf("SaveProposal: %v", err)
	}
	return a, store, target
}

func boolPtr(v bool) *bool { return &v }

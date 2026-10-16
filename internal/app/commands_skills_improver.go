package app

import (
	"fmt"

	"github.com/usewhale/whale/internal/plugins"
	"github.com/usewhale/whale/internal/plugins/skillsimprover"
)

// skillProposalResultKind is the LocalResult kind the confirmation prompt for
// a skill write renders.
const skillProposalResultKind = "skills-improver-decision"

// skillsImproverEnabled reports whether the user turned the plugin on. The
// plugin is default-disabled, so nothing runs before that (M1-36..M1-38).
func (a *App) skillsImproverEnabled() bool {
	if a == nil || a.pluginManager == nil {
		return false
	}
	st, ok := a.pluginManager.Status(skillsimprover.PluginID)
	return ok && st.Enabled
}

func (a *App) skillsImproverStore() *skillsimprover.Store {
	ctx := plugins.Context{DataDir: a.cfg.DataDir, WorkspaceRoot: a.workspaceRoot}
	return skillsimprover.StoreFor(skillsimprover.Context{
		DataDir:       ctx.PluginDataDir(skillsimprover.PluginID),
		WorkspaceRoot: a.workspaceRoot,
	})
}

// skillProposalDecision turns "/skills-improver apply|restore <id>" into a
// confirmation prompt. It writes nothing: only a confirmed prompt runs the
// write (M1-50, M1-51).
func (a *App) skillProposalDecision(line string) (CommandExecution, bool, error) {
	id, action, ok := skillsimprover.ParseDecision(line)
	if !ok || !a.skillsImproverEnabled() {
		return CommandExecution{}, false, nil
	}
	decision, err := a.skillsImproverStore().PendingDecision(id, action)
	if err != nil {
		return CommandExecution{Handled: true}, true, err
	}
	res := buildSkillProposalLocalResult(decision, a.sessionID)
	return CommandExecution{Handled: true, Text: res.PlainText, LocalResult: res}, true, nil
}

func buildSkillProposalLocalResult(d skillsimprover.Decision, sessionID string) *LocalResult {
	title := "Apply proposal " + d.ProposalID + "?"
	confirm := "Yes, apply it"
	if d.Action == skillsimprover.ActionRestore {
		title = "Restore proposal " + d.ProposalID + "?"
		confirm = "Yes, restore it"
	}
	approval := &SkillProposalApproval{
		ID:         d.ProposalID,
		Action:     d.Action,
		SessionID:  sessionID,
		Target:     d.Target,
		CurrentSHA: d.CurrentSHA,
		ResultSHA:  d.ResultSHA,
	}
	return &LocalResult{
		Kind:  skillProposalResultKind,
		Title: title,
		Fields: []LocalResultField{
			{Label: "Skill", Value: d.Skill, Tone: "info"},
			{Label: "Action", Value: d.Action},
			{Label: "Target", Value: d.Target},
			{Label: "Current SHA-256", Value: d.CurrentSHA},
			{Label: "Result SHA-256", Value: d.ResultSHA},
			{Label: "Summary", Value: d.Summary},
			{Label: "Risk", Value: d.Risk, Tone: "warn"},
		},
		Actions: []LocalResultAction{
			{Label: confirm, SkillProposal: approval},
			{Label: "No", Description: "Cancel; the skill file stays as it is", Tone: "muted"},
		},
		PlainText: d.DecisionText(),
	}
}

// ApplySkillProposal runs one confirmed skill write. The approval must match
// the proposal as it is on disk right now: a mismatch leaves the target as it
// was (M1-52).
func (a *App) ApplySkillProposal(approval SkillProposalApproval) (string, error) {
	if !a.skillsImproverEnabled() {
		return "", fmt.Errorf("skills-improver plugin is disabled")
	}
	p, err := a.skillsImproverStore().Decide(approval.ID, approval.Action, skillsimprover.Approval{
		SessionID:  approval.SessionID,
		Target:     approval.Target,
		CurrentSHA: approval.CurrentSHA,
		ResultSHA:  approval.ResultSHA,
	})
	if err != nil {
		return "", err
	}
	// The skill files on disk are the source of truth and every lookup re-reads
	// them, so the only state that can still carry the old content is the cached
	// agent built earlier in this session. Drop it: the next turn rebuilds its
	// toolset and system blocks from disk, and the prompt already handed to a
	// running turn is left alone (M1-56, M1-57, M1-58).
	a.resetAgent()
	if approval.Action == skillsimprover.ActionRestore {
		return fmt.Sprintf("restored skill %s to its original content (proposal %s)", p.Skill, p.ID), nil
	}
	return fmt.Sprintf("applied skill %s (proposal %s)", p.Skill, p.ID), nil
}

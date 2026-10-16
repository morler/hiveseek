package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/usewhale/whale/internal/runtime/protocol"
	tuirender "github.com/usewhale/whale/internal/tui/render"
)

func skillProposalTestResult(action string) *protocol.LocalResult {
	return &protocol.LocalResult{
		Kind:      "skills-improver-decision",
		Title:     "Apply this skill change?",
		PlainText: "skills-improver apply sp-1",
		Sections: []protocol.LocalResultSection{{
			Title:  "Proposed change",
			Fields: []protocol.LocalResultField{{Label: "Summary", Value: "make it specific"}},
		}},
		Actions: []protocol.LocalResultAction{
			{
				Label: "Apply",
				SkillProposal: &protocol.SkillProposalApproval{
					ID: "sp-1", Action: action, SessionID: "s1", Target: "/ws/.whale/skills/demo/SKILL.md",
					CurrentSHA: "aaaa", ResultSHA: "bbbb",
				},
			},
			{Label: "No"},
		},
	}
}

func TestSkillProposalResultOpensConfirmationModal(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 100, height: 30}

	m.handleLocalSubmitResultEvent(protocol.Event{Status: "info", Text: "skills-improver apply sp-1", LocalResult: skillProposalTestResult("apply")})

	if m.mode != modeWorkflowLaunch {
		t.Fatalf("mode = %v, want confirmation modal", m.mode)
	}
	if got := m.renderWorkflowLaunch(); !strings.Contains(got, "Apply this skill change?") {
		t.Fatalf("confirmation render missing title:\n%s", got)
	}
}

func TestSkillProposalConfirmDispatchesAndCancelDoesNot(t *testing.T) {
	intents := []protocol.Intent{}
	m := model{
		assembler: tuirender.NewAssembler(),
		mode:      modeWorkflowLaunch,
		width:     100,
		height:    30,
		workflowLaunch: struct {
			result    *protocol.LocalResult
			selected  int
			rawScroll int
		}{result: skillProposalTestResult("apply")},
		dispatch: func(in protocol.Intent) { intents = append(intents, in) },
	}

	if cmd := m.handleWorkflowLaunchKey(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil {
		t.Fatal("cancel should not run a command")
	}
	if m.mode != modeChat || len(intents) != 0 {
		t.Fatalf("cancel must write nothing: mode=%v intents=%+v", m.mode, intents)
	}

	m.workflowLaunch.result = skillProposalTestResult("apply")
	m.mode = modeWorkflowLaunch
	if cmd := m.handleWorkflowLaunchKey(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("confirm should dispatch, not run an async command")
	}
	if len(intents) != 1 {
		t.Fatalf("intents = %+v, want one", intents)
	}
	if intents[0].Kind != protocol.IntentSkillProposalApply || intents[0].SkillProposal == nil || intents[0].SkillProposal.ID != "sp-1" {
		t.Fatalf("unexpected intent: %+v", intents[0])
	}

	m.workflowLaunch.result = skillProposalTestResult("restore")
	m.mode = modeWorkflowLaunch
	intents = nil
	m.handleWorkflowLaunchKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(intents) != 1 || intents[0].Kind != protocol.IntentSkillProposalRestore {
		t.Fatalf("restore action should dispatch restore: %+v", intents)
	}
}

func TestSkillProposalErrorResultRendersReason(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 100, height: 30}

	m.handleLocalSubmitResultEvent(protocol.Event{
		Status: "error",
		Text:   "skill proposal sp-1: target_changed: target skill changed: sha mismatch",
	})

	got := strings.Join(tuirender.ChatLines(m.transcript, 120), "\n")
	if !strings.Contains(got, "target_changed") || !strings.Contains(got, "sha mismatch") {
		t.Fatalf("error interface should show the actionable reason:\n%s", got)
	}
	if m.mode != modeChat {
		t.Fatalf("an error must not leave the confirmation modal open: %v", m.mode)
	}
}

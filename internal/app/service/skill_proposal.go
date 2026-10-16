package service

import "github.com/usewhale/whale/internal/app"

// runSkillProposalDecision performs one user-confirmed skill write and reports
// the outcome. The approval came from the confirmation prompt, so a bare
// command line, tool stdin or the environment cannot approve a write (M1-50,
// M1-51, M1-53).
func (s *Service) runSkillProposalDecision(approval *app.SkillProposalApproval) {
	if approval == nil {
		s.emit(Event{Kind: EventLocalSubmitResult, Status: "error", Text: "skill write: missing confirmation"})
		return
	}
	text, err := s.app.ApplySkillProposal(*approval)
	if err != nil {
		s.emit(Event{Kind: EventLocalSubmitResult, Status: "error", Text: err.Error()})
		return
	}
	s.emit(Event{Kind: EventLocalSubmitResult, Status: "ok", Text: text})
}

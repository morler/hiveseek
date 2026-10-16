package skillsimprover

import (
	"fmt"
	"strings"
)

// Skill write actions. Both are user commands: the agent gets no tool for
// them (M1-46).
const (
	ActionApply   = "apply"
	ActionRestore = "restore"
)

// Approval is the user's direct confirmation of one skill write, taken from
// the app's confirmation channel. It is never built from tool input, from the
// environment or from anything a candidate wrote (M1-50, M1-53). CurrentSHA
// is the hash of the target the user was shown, ResultSHA the hash it becomes;
// for an apply those are the proposal's original and proposed hashes (M1-52).
type Approval struct {
	SessionID  string
	Target     string
	CurrentSHA string
	ResultSHA  string
}

// Decision describes what one approval would write. It is produced without
// touching the target, so the caller can show the user what will happen.
type Decision struct {
	ProposalID string
	Action     string
	Skill      string
	Target     string
	CurrentSHA string
	ResultSHA  string
	Summary    string
	Risk       string
}

// ParseDecision matches the mutating command forms only:
// "/skills-improver apply <id>" and "/skills-improver restore <id>".
func ParseDecision(line string) (id, action string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 3 || fields[0] != "/"+PluginID {
		return "", "", false
	}
	if fields[1] != ActionApply && fields[1] != ActionRestore {
		return "", "", false
	}
	return fields[2], fields[1], true
}

// PendingDecision reads the proposal and reports what the action would write.
// It refuses states the action cannot move, and writes nothing.
func (s *Store) PendingDecision(id, action string) (Decision, error) {
	p, err := s.ReadProposal(id)
	if err != nil {
		return Decision{}, err
	}
	if err := checkActionState(p, action); err != nil {
		return Decision{}, err
	}
	decision := Decision{
		ProposalID: p.ID,
		Action:     action,
		Skill:      p.Skill,
		Target:     p.SkillFilePath,
		CurrentSHA: p.OriginalSHA256,
		ResultSHA:  sha256Hex(proposedSkillBytes(p)),
		Summary:    p.Summary,
		Risk:       p.Risk,
	}
	if action == ActionRestore {
		decision.CurrentSHA = p.AppliedSkillSHA256
		decision.ResultSHA = p.OriginalSHA256
	}
	return decision, nil
}

// Decide carries out the approved action. The approval must match the
// proposal as it is on disk right now: a missing session, a different target
// or any drift in either hash rejects the write (M1-52).
func (s *Store) Decide(id, action string, ap Approval) (Proposal, error) {
	decision, err := s.PendingDecision(id, action)
	if err != nil {
		return Proposal{}, err
	}
	if err := ap.matches(decision); err != nil {
		return Proposal{}, err
	}
	if action == ActionRestore {
		return s.RestoreProposal(id)
	}
	return s.ApplyProposal(id)
}

func checkActionState(p Proposal, action string) error {
	state := p.effectiveState()
	switch action {
	case ActionApply:
		if state == StateProposed || state == StateApplying {
			return nil
		}
	case ActionRestore:
		if state == StateApplied || state == StateApplying {
			return nil
		}
	default:
		return reject(RejectProposalIncomplete, "unknown skill action %q", action)
	}
	return reject(RejectProposalIncomplete, "proposal is %s, cannot %s it", state, action)
}

// matches compares the approval with what the user was shown.
func (ap Approval) matches(d Decision) error {
	if strings.TrimSpace(ap.SessionID) == "" {
		return reject(RejectApprovalInvalid, "approval is not bound to a session")
	}
	if ap.Target != d.Target || ap.CurrentSHA != d.CurrentSHA || ap.ResultSHA != d.ResultSHA {
		return reject(RejectApprovalInvalid, "approval does not match proposal %s", d.ProposalID)
	}
	return nil
}

// DecisionText renders the pending decision for the confirmation prompt.
func (d Decision) DecisionText() string {
	lines := []string{
		fmt.Sprintf("Confirm %s of proposal %s?", d.Action, d.ProposalID),
		"",
		"skill: " + d.Skill,
		"target: " + d.Target,
		"current sha256: " + d.CurrentSHA,
		"result sha256: " + d.ResultSHA,
		"summary: " + d.Summary,
	}
	if d.Risk != "" {
		lines = append(lines, "risk: "+d.Risk)
	}
	lines = append(lines, "", "The skill file is replaced only after you confirm here.")
	return strings.Join(lines, "\n")
}

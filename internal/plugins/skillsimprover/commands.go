package skillsimprover

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// CommandUsage is the frozen command surface (M0-E). Apply and restore are
// user commands only: the agent gets no tool for them (M1-46).
const CommandUsage = "/skills-improver [status|proposals|show <id>|apply <id>|restore <id>]"

// HandleCommand runs one /skills-improver line. It reports whether it changed
// anything on disk so the caller can refresh session state.
func HandleCommand(store *Store, line string) (string, bool, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 || fields[0] != "/"+PluginID {
		return "", false, fmt.Errorf("usage: %s", CommandUsage)
	}
	if len(fields) == 1 || fields[1] == "status" {
		if len(fields) > 2 {
			return "", false, fmt.Errorf("usage: %s", CommandUsage)
		}
		out, err := statusText(store)
		return out, false, err
	}
	if fields[1] == "proposals" {
		if len(fields) > 2 {
			return "", false, fmt.Errorf("usage: %s", CommandUsage)
		}
		out, err := proposalsText(store)
		return out, false, err
	}
	if fields[1] == "show" {
		if len(fields) != 3 {
			return "", false, fmt.Errorf("usage: %s", CommandUsage)
		}
		out, err := showText(store, fields[2])
		return out, false, err
	}
	if fields[1] == ActionApply || fields[1] == ActionRestore {
		// The write runs only through the app's confirmation prompt: a bare
		// command line (agent, script, tool stdin) cannot approve it (M1-50).
		return "", false, fmt.Errorf("%s <id> needs the confirmation prompt: %s", fields[1], CommandUsage)
	}
	return "", false, fmt.Errorf("usage: %s", CommandUsage)
}

// maxShowDiffLines bounds what /skills-improver show prints. Skill files are
// small; a runaway diff means something else went wrong.
//
// shortcut: the diff is truncated, not paginated. Add paging if proposals
// ever carry files this large.
const maxShowDiffLines = 400

func showText(store *Store, id string) (string, error) {
	p, err := store.ReadProposal(id)
	if err != nil {
		return "", err
	}
	original, err := store.proposalOriginal(p)
	if err != nil {
		return "", err
	}
	lines := []string{
		"proposal " + p.ID,
		"state: " + p.effectiveState(),
		"skill: " + p.Skill,
		"target: " + p.SkillFilePath,
		"original sha256: " + p.OriginalSHA256,
		"proposed sha256: " + sha256Hex(proposedSkillBytes(p)),
		"summary: " + p.Summary,
	}
	if p.Risk != "" {
		lines = append(lines, "risk: "+p.Risk)
	}
	lines = append(lines, "", "--- original", "+++ proposed")
	diff := diffLines(original, p.ProposedSkillMD)
	if len(diff) > maxShowDiffLines {
		diff = append(diff[:maxShowDiffLines], fmt.Sprintf("... %d more line(s)", len(diff)-maxShowDiffLines))
	}
	return strings.Join(append(lines, diff...), "\n"), nil
}

// proposalOriginal returns the pre-apply bytes: the private backup when it is
// still intact, otherwise whatever the target holds now.
func (s *Store) proposalOriginal(p Proposal) (string, error) {
	if p.OriginalBackupPath != "" {
		if data, err := os.ReadFile(p.OriginalBackupPath); err == nil && sha256Hex(data) == p.OriginalSHA256 {
			return string(data), nil
		}
	}
	data, err := os.ReadFile(p.SkillFilePath)
	if err != nil {
		return "", fmt.Errorf("read target %s: %w", p.SkillFilePath, err)
	}
	return string(data), nil
}

// diffLines renders a line-level longest-common-subsequence diff: context
// lines keep a leading space, removals a "-", additions a "+".
func diffLines(original, proposed string) []string {
	a := splitDiffLines(original)
	b := splitDiffLines(proposed)
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
				continue
			}
			table[i][j] = max(table[i+1][j], table[i][j+1])
		}
	}
	out := make([]string, 0, len(a)+len(b))
	for i, j := 0, 0; i < len(a) || j < len(b); {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			out = append(out, " "+a[i])
			i++
			j++
		case j < len(b) && (i == len(a) || table[i][j+1] > table[i+1][j]):
			out = append(out, "+"+b[j])
			j++
		default:
			out = append(out, "-"+a[i])
			i++
		}
	}
	return out
}

func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func proposalsText(store *Store) (string, error) {
	proposals, err := store.ListProposals()
	if err != nil {
		return "", err
	}
	if len(proposals) == 0 {
		return "no proposals", nil
	}
	sort.SliceStable(proposals, func(i, j int) bool { return proposals[i].CreatedAt.After(proposals[j].CreatedAt) })
	lines := make([]string, 0, len(proposals)+1)
	lines = append(lines, fmt.Sprintf("%d proposal(s)", len(proposals)))
	for _, p := range proposals {
		lines = append(lines, fmt.Sprintf("  %s  %s  %s", p.ID, p.effectiveState(), p.Skill))
	}
	return strings.Join(lines, "\n"), nil
}

func statusText(store *Store) (string, error) {
	status, err := store.Status()
	if err != nil {
		return "", err
	}
	lines := []string{
		PluginID,
		// The command only exists while the plugin is loaded, so this bit is
		// known to be true whenever the command runs (M1-47).
		"enabled: yes",
		fmt.Sprintf("evidence: %d record(s)", status.EvidenceCount),
		fmt.Sprintf("proposals: %d", status.ProposalCount),
		"collection: " + OnOffText(status.CollectionEnabled),
		"data: " + status.DataDir,
	}
	if status.WriteError != "" {
		lines = append(lines, "write error: "+status.WriteError)
	}
	if len(status.Recent) > 0 {
		lines = append(lines, "", "recent evidence:")
		recent := append([]Evidence(nil), status.Recent...)
		sort.SliceStable(recent, func(i, j int) bool { return recent[i].CreatedAt.After(recent[j].CreatedAt) })
		for _, ev := range recent {
			lines = append(lines, "  "+evidenceLine(ev))
		}
	}
	return strings.Join(lines, "\n"), nil
}

func evidenceLine(ev Evidence) string {
	parts := []string{ev.CreatedAt.UTC().Format("2006-01-02 15:04:05"), ev.Kind}
	if ev.Skill != "" {
		parts = append(parts, ev.Skill)
	}
	if ev.ErrorCode != "" {
		parts = append(parts, ev.ErrorCode)
	}
	if ev.Summary != "" {
		parts = append(parts, ev.Summary)
	}
	return strings.Join(parts, "  ")
}

// OnOffText renders a boolean for the plain-text command output.
func OnOffText(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

package skillsimprover

import (
	"fmt"
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
	return "", false, fmt.Errorf("usage: %s", CommandUsage)
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

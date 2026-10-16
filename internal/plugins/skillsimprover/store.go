package skillsimprover

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/usewhale/whale/internal/securefs"
	"github.com/usewhale/whale/internal/skills"
)

const (
	PluginID         = "skills-improver"
	evidenceFileName = "evidence.jsonl"
	// maxEvidenceSummaryLen caps the one summary line an evidence record may
	// carry: the shortest text that still says what went wrong.
	maxEvidenceSummaryLen = 200
	maxProposalTextLen    = 200000
)

// Proposal lifecycle states. A proposal without a state predates the state
// machine: its applied_at timestamp decides, so old files keep their meaning.
const (
	StateProposed = "proposed"
	StateApplying = "applying"
	StateApplied  = "applied"
	StateRejected = "rejected"
	StateRestored = "restored"
)

// proposalSchemaVersion is the schema of the files this build writes. Files
// without the field are older and still readable.
const proposalSchemaVersion = 1

// Rejection codes for skill write targets and unusable proposal files. Callers
// distinguish a refused write by code instead of matching error text.
const (
	// RejectTargetOutsideProjectSkillRoots means the target is not inside the
	// project-level `.whale/skills` or `.agents/skills` roots.
	RejectTargetOutsideProjectSkillRoots = "target_outside_project_skill_roots"
	// RejectTargetHasSymlink means a path element of the target below the project
	// skill root is a symbolic link.
	RejectTargetHasSymlink = "target_has_symlink"
	// RejectTargetNotRegularFile means the target is not an existing regular file.
	RejectTargetNotRegularFile = "target_not_regular_file"
	// RejectProposalSchemaUnsupported means the proposal file declares a schema
	// this build does not know.
	RejectProposalSchemaUnsupported = "proposal_schema_unsupported"
	// RejectProposalDamaged means the proposal file cannot be parsed.
	RejectProposalDamaged = "proposal_damaged"
	// RejectProposalIncomplete means the proposal lacks the material an apply or
	// a restore needs, such as the recorded original hash or the backup.
	RejectProposalIncomplete = "proposal_incomplete"
	// RejectProposalIDInvalid means the id is not a plain identifier, so it could
	// escape the proposals directory if used as a file name.
	RejectProposalIDInvalid = "proposal_id_invalid"
	// RejectApprovalInvalid means the user approval does not match the proposal
	// on disk, so nothing may be written (M1-52).
	RejectApprovalInvalid = "approval_invalid"
	// RejectLogDamaged means the evidence log holds an unparsable line.
	RejectLogDamaged = "log_damaged"
	// RejectRestoreConflict means the target changed after the apply, so putting
	// the original back would discard a human edit.
	RejectRestoreConflict = "restore_conflict"
	// RejectRecoveryConflict means an interrupted apply left a target that matches
	// neither the original nor the proposed bytes.
	RejectRecoveryConflict = "recovery_conflict"
	// RejectTargetDrifted means the target no longer holds the bytes a proposal
	// was written against, because something else edited the file. Nothing that
	// runs outside this process can be locked out, so the change is reported
	// instead of being overwritten (M1-24).
	RejectTargetDrifted = "target_changed"
)

// RejectError is a refused write target carrying a stable code.
type RejectError struct {
	Code   string
	Reason string
}

func (e *RejectError) Error() string { return e.Code + ": " + e.Reason }

func reject(code, format string, args ...any) error {
	return &RejectError{Code: code, Reason: fmt.Sprintf(format, args...)}
}

// restoreOriginalBytes puts the recorded original back on the target. The backup
// is verified against the recorded hash first, so a damaged backup never lands.
func (s *Store) restoreOriginalBytes(p Proposal) error {
	path := strings.TrimSpace(p.OriginalBackupPath)
	if path == "" {
		return reject(RejectProposalIncomplete, "proposal %s has no original backup", p.ID)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return reject(RejectProposalIncomplete, "read original backup for %s: %v", p.ID, err)
	}
	if sha256Hex(b) != p.OriginalSHA256 {
		return reject(RejectProposalIncomplete, "original backup for %s no longer matches the recorded hash", p.ID)
	}
	return writeFileAtomic(p.SkillFilePath, b)
}

type Store struct {
	dataDir       string
	workspaceRoot string

	// mu guards lastWriteErr, which keeps an evidence write failure visible in
	// status instead of dropping it (M1-30).
	mu           sync.Mutex
	lastWriteErr error
}

func (s *Store) setWriteError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastWriteErr = err
}

// EvidenceWriteError reports the last failed evidence append, if any.
func (s *Store) EvidenceWriteError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastWriteErr == nil {
		return ""
	}
	return s.lastWriteErr.Error()
}

// RecordEvidence appends an evidence record for the hook path: a failure is
// kept in the store status rather than discarded (M1-30).
func (s *Store) RecordEvidence(ev Evidence) {
	if !s.CollectionEnabled() {
		return
	}
	if _, err := s.AppendEvidence(ev); err != nil {
		s.setWriteError(err)
		return
	}
	s.setWriteError(nil)
}

// Evidence is one minimized record of a skill improvement signal. This struct
// is the whole of what may reach disk: the skill, the failure code, the
// shortest summary, the session it belongs to and the record id. User prompts,
// tool arguments, tool results, model output, environment values and
// credentials are never stored, so these fields are the privacy boundary
// itself -- do not add one without revisiting that boundary.
type Evidence struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Kind      string    `json:"kind"`
	SessionID string    `json:"session_id,omitempty"`
	Skill     string    `json:"skill,omitempty"`
	ErrorCode string    `json:"error_code,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}

type Proposal struct {
	SchemaVersion  int       `json:"schema_version,omitempty"`
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	Skill          string    `json:"skill"`
	SkillFilePath  string    `json:"skill_file_path"`
	OriginalSHA256 string    `json:"original_sha256"`
	// OriginalBackupPath is where the pre-apply bytes were kept; empty on
	// proposals written before backups existed.
	OriginalBackupPath string    `json:"original_backup_path,omitempty"`
	Summary            string    `json:"summary"`
	Risk               string    `json:"risk,omitempty"`
	ProposedSkillMD    string    `json:"proposed_skill_md"`
	EvidenceIDs        []string  `json:"evidence_ids,omitempty"`
	AppliedAt          time.Time `json:"applied_at,omitempty"`
	AppliedSkillSHA256 string    `json:"applied_skill_sha256,omitempty"`
	State              string    `json:"state,omitempty"`
	RestoredAt         time.Time `json:"restored_at,omitempty"`
}

// effectiveState resolves the state of a proposal written before the state
// machine existed: no state plus an applied timestamp means applied.
func (p Proposal) effectiveState() string {
	if p.State != "" {
		return p.State
	}
	if !p.AppliedAt.IsZero() {
		return StateApplied
	}
	return StateProposed
}

type Status struct {
	EvidenceCount int
	ProposalCount int
	DataDir       string
	Recent        []Evidence
	// CollectionEnabled is false when the user turned evidence collection off.
	CollectionEnabled bool
	// WriteError holds the last failed evidence append, so a write failure is
	// visible in status instead of being dropped (M1-30).
	WriteError string
}

func NewStore(dataDir, workspaceRoot string) *Store {
	return &Store{dataDir: strings.TrimSpace(dataDir), workspaceRoot: strings.TrimSpace(workspaceRoot)}
}

func (s *Store) DataDir() string {
	return s.dataDir
}

func (s *Store) AppendEvidence(ev Evidence) (Evidence, error) {
	if strings.TrimSpace(s.dataDir) == "" {
		return Evidence{}, fmt.Errorf("skills-improver data dir is empty")
	}
	if err := securefs.MkdirPrivate(s.dataDir); err != nil {
		return Evidence{}, fmt.Errorf("create skills-improver data dir: %w", err)
	}
	ev.CreatedAt = time.Now().UTC()
	ev.Kind = cleanToken(ev.Kind)
	ev.Skill = cleanSkillName(ev.Skill)
	ev.Summary = truncate(redactSecrets(firstLine(ev.Summary)), maxEvidenceSummaryLen)
	if ev.ID == "" {
		ev.ID = evidenceID(ev)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return Evidence{}, err
	}
	f, err := securefs.OpenPrivateAppend(s.evidencePath())
	if err != nil {
		return Evidence{}, fmt.Errorf("open evidence log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return Evidence{}, fmt.Errorf("write evidence log: %w", err)
	}
	return ev, nil
}

func (s *Store) ListEvidence(skill string, limit int) ([]Evidence, error) {
	if limit <= 0 {
		limit = 20
	}
	all, err := s.readEvidence()
	if err != nil {
		return nil, err
	}
	skill = cleanSkillName(skill)
	filtered := make([]Evidence, 0, len(all))
	for _, ev := range all {
		if skill == "" || ev.Skill == skill {
			filtered = append(filtered, ev)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].CreatedAt.After(filtered[j].CreatedAt) })
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// ClearEvidence removes every stored record, or only those for one skill, and
// requires the caller to pass an explicit confirmation (M1-32).
func (s *Store) ClearEvidence(skill string, confirmed bool) (int, error) {
	if !confirmed {
		return 0, fmt.Errorf("clearing evidence requires explicit confirmation")
	}
	all, err := s.readEvidence()
	if err != nil {
		return 0, err
	}
	skill = cleanSkillName(skill)
	kept := make([]Evidence, 0, len(all))
	removed := 0
	for _, ev := range all {
		if skill == "" || ev.Skill == skill {
			removed++
			continue
		}
		kept = append(kept, ev)
	}
	if removed == 0 {
		return 0, nil
	}
	var buf bytes.Buffer
	for _, ev := range kept {
		b, err := json.Marshal(ev)
		if err != nil {
			return 0, err
		}
		buf.Write(append(b, '\n'))
	}
	if err := securefs.WritePrivateFile(s.evidencePath(), buf.Bytes()); err != nil {
		return 0, fmt.Errorf("rewrite evidence log: %w", err)
	}
	return removed, nil
}

// collectionDisabledMarker is the file whose presence turns collection off.
const collectionDisabledMarker = "collection.disabled"

// CollectionEnabled reports whether new evidence is collected. Collection is on
// unless the user turned it off (M1-33).
func (s *Store) CollectionEnabled() bool {
	if strings.TrimSpace(s.dataDir) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.dataDir, collectionDisabledMarker))
	return os.IsNotExist(err)
}

// SetCollectionEnabled turns collection on or off. The choice lives in the
// private data dir, so it survives a restart (M1-33).
func (s *Store) SetCollectionEnabled(enabled bool) error {
	if strings.TrimSpace(s.dataDir) == "" {
		return fmt.Errorf("skills-improver data dir is empty")
	}
	path := filepath.Join(s.dataDir, collectionDisabledMarker)
	if enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("resume evidence collection: %w", err)
		}
		return nil
	}
	return securefs.WritePrivateFile(path, []byte("collection disabled\n"))
}

func (s *Store) Status() (Status, error) {
	all, err := s.readEvidence()
	if err != nil {
		return Status{}, err
	}
	proposals, err := s.ListProposals()
	if err != nil {
		return Status{}, err
	}
	recent := append([]Evidence(nil), all...)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].CreatedAt.After(recent[j].CreatedAt) })
	if len(recent) > 5 {
		recent = recent[:5]
	}
	return Status{
		EvidenceCount:     len(all),
		ProposalCount:     len(proposals),
		DataDir:           s.dataDir,
		Recent:            recent,
		CollectionEnabled: s.CollectionEnabled(),
		WriteError:        s.EvidenceWriteError(),
	}, nil
}

func (s *Store) SaveProposal(p Proposal) (Proposal, error) {
	if strings.TrimSpace(s.dataDir) == "" {
		return Proposal{}, fmt.Errorf("skills-improver data dir is empty")
	}
	p.ID = ""
	p.Skill = strings.TrimSpace(p.Skill)
	p.SkillFilePath = strings.TrimSpace(p.SkillFilePath)
	p.Summary = truncate(p.Summary, maxProposalTextLen)
	p.Risk = truncate(p.Risk, maxProposalTextLen)
	p.ProposedSkillMD = strings.TrimSpace(p.ProposedSkillMD)
	if p.Skill == "" {
		return Proposal{}, fmt.Errorf("skill is required")
	}
	if p.SkillFilePath == "" {
		return Proposal{}, fmt.Errorf("skill_file_path is required")
	}
	if strings.HasPrefix(p.SkillFilePath, "plugin://") {
		return Proposal{}, fmt.Errorf("plugin skills cannot be applied in phase 1")
	}
	if len(p.ProposedSkillMD) == 0 {
		return Proposal{}, fmt.Errorf("proposed_skill_md is required")
	}
	if len(p.ProposedSkillMD) > maxProposalTextLen {
		return Proposal{}, fmt.Errorf("proposed_skill_md is too large")
	}
	if err := validateSkillContent(p.Skill, p.ProposedSkillMD); err != nil {
		return Proposal{}, err
	}
	abs, err := filepath.Abs(p.SkillFilePath)
	if err != nil {
		return Proposal{}, err
	}
	if err := requireKnownSkillPath(s.workspaceRoot, p.Skill, abs); err != nil {
		return Proposal{}, err
	}
	current, err := os.ReadFile(abs)
	if err != nil {
		return Proposal{}, fmt.Errorf("read target skill: %w", err)
	}
	currentSHA := sha256Hex(current)
	if strings.TrimSpace(p.OriginalSHA256) == "" {
		return Proposal{}, fmt.Errorf("original_sha256 is required")
	}
	if p.OriginalSHA256 != currentSHA {
		return Proposal{}, reject(RejectTargetDrifted, "target skill changed: sha mismatch")
	}
	p.SkillFilePath = abs
	p.CreatedAt = time.Now().UTC()
	p.ID = proposalID(p)
	p.State = StateProposed
	p.SchemaVersion = proposalSchemaVersion
	if err := s.writeProposal(p); err != nil {
		return Proposal{}, fmt.Errorf("write proposal: %w", err)
	}
	return p, nil
}

func (s *Store) ListProposals() ([]Proposal, error) {
	dir := s.proposalsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Proposal, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		p, err := s.ReadProposal(strings.TrimSuffix(ent.Name(), ".json"))
		if err == nil {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ReadProposal(id string) (Proposal, error) {
	// The id becomes a file name, so anything that is not already a plain
	// identifier is refused instead of being silently rewritten (M1-21).
	if id == "" || cleanID(id) != strings.TrimSpace(id) {
		return Proposal{}, reject(RejectProposalIDInvalid, "invalid proposal id %q", id)
	}
	b, err := os.ReadFile(filepath.Join(s.proposalsDir(), id+".json"))
	if err != nil {
		return Proposal{}, err
	}
	var p Proposal
	if err := json.Unmarshal(b, &p); err != nil {
		return Proposal{}, reject(RejectProposalDamaged, "proposal %s is unreadable: %v", id, err)
	}
	if p.SchemaVersion > proposalSchemaVersion {
		return Proposal{}, reject(RejectProposalSchemaUnsupported, "proposal %s declares schema_version %d, this build understands %d", id, p.SchemaVersion, proposalSchemaVersion)
	}
	return p, nil
}

func (s *Store) ApplyProposal(id string) (Proposal, error) {
	first, err := s.ReadProposal(id)
	if err != nil {
		return Proposal{}, err
	}
	// Recoveries and applies share the target lock, so no two spellings of the
	// same file can read the target and write it back interleaved (M1-22).
	return s.lockedTarget(first.SkillFilePath, func() (Proposal, error) {
		return s.applyProposalLocked(id)
	})
}

func (s *Store) applyProposalLocked(id string) (Proposal, error) {
	p, err := s.ReadProposal(id)
	if err != nil {
		return Proposal{}, err
	}
	switch state := p.effectiveState(); state {
	case StateProposed:
		// continue below
	case StateApplying:
		// An earlier apply was interrupted between the state commit and the
		// applied log; settle it, with the lock already held (M1-16).
		return s.recoverInterruptedApply(p)
	default:
		return Proposal{}, fmt.Errorf("proposal is %s, not proposed", state)
	}
	if strings.HasPrefix(p.SkillFilePath, "plugin://") {
		return Proposal{}, fmt.Errorf("plugin skills cannot be applied in phase 1")
	}
	if err := validateSkillContent(p.Skill, p.ProposedSkillMD); err != nil {
		return Proposal{}, err
	}
	// A proposal written before the transaction fields existed carries no
	// recoverable original, so it is not applied (M1-21b).
	if p.SchemaVersion == 0 || strings.TrimSpace(p.OriginalSHA256) == "" {
		return Proposal{}, reject(RejectProposalIncomplete, "proposal %s predates backups and has no transaction material; re-verify and re-save it before applying", p.ID)
	}
	if err := requireKnownSkillPath(s.workspaceRoot, p.Skill, p.SkillFilePath); err != nil {
		return Proposal{}, err
	}
	current, err := os.ReadFile(p.SkillFilePath)
	if err != nil {
		return Proposal{}, fmt.Errorf("read target skill: %w", err)
	}
	if sha256Hex(current) != p.OriginalSHA256 {
		return Proposal{}, reject(RejectTargetDrifted, "target skill changed: sha mismatch")
	}
	// The backup lands before the target is replaced; if it cannot be written
	// the target is left untouched. The applying state is committed first so a
	// crash mid-replace leaves an identifiable intermediate state on disk.
	if err := s.writeOriginalBackupInto(&p, current); err != nil {
		return Proposal{}, err
	}
	p.State = StateApplying
	if err := s.writeProposal(p); err != nil {
		return Proposal{}, fmt.Errorf("mark proposal applying: %w", err)
	}
	if err := writeFileAtomic(p.SkillFilePath, proposedSkillBytes(p)); err != nil {
		return Proposal{}, fmt.Errorf("write target skill: %w", err)
	}
	// The applied log is committed last.
	p.State = StateApplied
	p.AppliedAt = time.Now().UTC()
	p.AppliedSkillSHA256 = sha256Hex(proposedSkillBytes(p))
	if err := s.writeProposal(p); err != nil {
		return Proposal{}, fmt.Errorf("mark proposal applied: %w", err)
	}
	return p, nil
}

// recoverInterruptedApply finishes or undoes an apply that stopped after the
// applying state was committed. It decides from the target bytes alone, so it
// never writes the same change twice (M1-16, M1-17).
func (s *Store) recoverInterruptedApply(p Proposal) (Proposal, error) {
	current, err := os.ReadFile(p.SkillFilePath)
	if err != nil {
		return Proposal{}, fmt.Errorf("read target skill: %w", err)
	}
	proposed := []byte(strings.TrimSpace(p.ProposedSkillMD) + "\n")
	proposedSHA := sha256Hex(proposed)
	switch sha256Hex(current) {
	case proposedSHA:
		// The replace completed; only the applied log was lost.
		p.State = StateApplied
		p.AppliedAt = time.Now().UTC()
		p.AppliedSkillSHA256 = proposedSHA
		if err := s.writeProposal(p); err != nil {
			return Proposal{}, fmt.Errorf("mark proposal applied: %w", err)
		}
		return p, nil
	case p.OriginalSHA256:
		if err := s.restoreOriginalBytes(p); err != nil {
			return Proposal{}, err
		}
		p.State = StateRestored
		p.RestoredAt = time.Now().UTC()
		if err := s.writeProposal(p); err != nil {
			return Proposal{}, fmt.Errorf("mark proposal restored: %w", err)
		}
		return p, nil
	default:
		return Proposal{}, reject(RejectRecoveryConflict, "target %s matches neither the original nor the proposed bytes; resolve it by hand", p.SkillFilePath)
	}
}

// PruneProposal marks a proposal rejected. Nothing is written to the target.
func (s *Store) PruneProposal(id string) (Proposal, error) {
	p, err := s.ReadProposal(id)
	if err != nil {
		return Proposal{}, err
	}
	if state := p.effectiveState(); state == StateApplied || state == StateApplying {
		return Proposal{}, fmt.Errorf("proposal is %s and cannot be rejected", state)
	}
	p.State = StateRejected
	if err := s.writeProposal(p); err != nil {
		return Proposal{}, fmt.Errorf("mark proposal rejected: %w", err)
	}
	return p, nil
}

func (s *Store) readEvidence() ([]Evidence, error) {
	f, err := os.Open(s.evidencePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Evidence
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var ev Evidence
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			// A damaged log stops the read instead of silently dropping lines.
			return nil, reject(RejectLogDamaged, "evidence log line is unreadable: %v", err)
		}
		out = append(out, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, reject(RejectLogDamaged, "read evidence log: %v", err)
	}
	return out, nil
}

func (s *Store) evidencePath() string {
	return filepath.Join(s.dataDir, evidenceFileName)
}

func (s *Store) proposalsDir() string {
	return filepath.Join(s.dataDir, "proposals")
}

func (s *Store) writeProposal(p Proposal) error {
	if err := securefs.MkdirPrivate(s.proposalsDir()); err != nil {
		return fmt.Errorf("create proposals dir: %w", err)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return securefs.WritePrivateFile(filepath.Join(s.proposalsDir(), p.ID+".json"), append(b, '\n'))
}

func (s *Store) backupsDir() string {
	return filepath.Join(s.dataDir, "backups")
}

// writeOriginalBackupInto stores the skill bytes read at apply time, named by
// their sha256, so the exact original is recoverable before the target is
// replaced. Content addressed: a retry after a failed apply does not rewrite it.
func (s *Store) writeOriginalBackupInto(p *Proposal, content []byte) error {
	if err := securefs.MkdirPrivate(s.backupsDir()); err != nil {
		return fmt.Errorf("create backups dir: %w", err)
	}
	path := filepath.Join(s.backupsDir(), "sha256-"+sha256Hex(content)+".md")
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
		if err := securefs.WritePrivateFile(path, content); err != nil {
			return fmt.Errorf("write original backup: %w", err)
		}
	}
	p.OriginalBackupPath = path
	return nil
}

func validateSkillContent(expectedName, content string) error {
	s, err := skills.ParseContent([]byte(content))
	if err != nil {
		return fmt.Errorf("invalid proposed SKILL.md: %w", err)
	}
	if err := s.Validate(); err != nil {
		return fmt.Errorf("invalid proposed SKILL.md: %w", err)
	}
	if cleanSkillName(s.Name) != cleanSkillName(expectedName) {
		return fmt.Errorf("proposed skill name mismatch: got %s want %s", s.Name, expectedName)
	}
	return nil
}

// RestoreProposal puts the recorded original back and marks the proposal
// restored. The target must still hold the bytes this apply wrote: anything else
// is a later edit and is refused rather than overwritten (M1-20).
func (s *Store) RestoreProposal(id string) (Proposal, error) {
	first, err := s.ReadProposal(id)
	if err != nil {
		return Proposal{}, err
	}
	return s.lockedTarget(first.SkillFilePath, func() (Proposal, error) {
		return s.restoreProposalLocked(id)
	})
}

func (s *Store) restoreProposalLocked(id string) (Proposal, error) {
	p, err := s.ReadProposal(id)
	if err != nil {
		return Proposal{}, err
	}
	switch state := p.effectiveState(); state {
	case StateApplied:
		// continue below
	case StateRestored:
		return p, nil // already restored; nothing to write
	case StateApplying:
		return s.recoverInterruptedApply(p)
	default:
		return Proposal{}, fmt.Errorf("proposal is %s and has nothing to restore", state)
	}
	if strings.TrimSpace(p.AppliedSkillSHA256) == "" {
		return Proposal{}, reject(RejectProposalIncomplete, "proposal %s records no applied hash, so a restore cannot be verified", p.ID)
	}
	if err := requireKnownSkillPath(s.workspaceRoot, p.Skill, p.SkillFilePath); err != nil {
		return Proposal{}, err
	}
	current, err := os.ReadFile(p.SkillFilePath)
	if err != nil {
		return Proposal{}, fmt.Errorf("read target skill: %w", err)
	}
	if sha256Hex(current) != p.AppliedSkillSHA256 {
		return Proposal{}, reject(RejectRestoreConflict, "target %s no longer holds the applied bytes; refusing to overwrite the newer content", p.SkillFilePath)
	}
	if err := s.restoreOriginalBytes(p); err != nil {
		return Proposal{}, err
	}
	p.State = StateRestored
	p.RestoredAt = time.Now().UTC()
	if err := s.writeProposal(p); err != nil {
		return Proposal{}, fmt.Errorf("mark proposal restored: %w", err)
	}
	return p, nil
}

func requireKnownSkillPath(workspaceRoot, skillName, path string) error {
	abs, err := normalizedExistingPath(path)
	if err != nil {
		return err
	}
	if err := requireInsideProjectSkillRoot(workspaceRoot, abs); err != nil {
		return err
	}
	// The symlink walk must see the caller's spelling, before symlinks are resolved.
	if err := requireNoSymlinkBelowRoot(workspaceRoot, path); err != nil {
		return err
	}
	if err := requireRegularFile(abs); err != nil {
		return err
	}
	roots := skills.DefaultRoots(workspaceRoot)
	found, ok := findSkillCaseInsensitive(roots, skillName)
	if !ok {
		return fmt.Errorf("skill not found in filesystem roots: %s", skillName)
	}
	foundPath, err := normalizedExistingPath(found.SkillFilePath)
	if err != nil {
		return err
	}
	if abs != foundPath {
		return fmt.Errorf("target path does not match discovered skill path")
	}
	return nil
}

// projectSkillRoots returns the only two directories a write target may live in:
// the project-level `.whale/skills` and `.agents/skills`. Home roots are read-only.
func projectSkillRoots(workspaceRoot string) []string {
	root := filepath.Clean(strings.TrimSpace(workspaceRoot))
	if root == "" || root == "." {
		return nil
	}
	dirs := []string{
		filepath.Join(root, ".whale", "skills"),
		filepath.Join(root, ".agents", "skills"),
	}
	for i, dir := range dirs {
		dirs[i] = resolvePathLenient(dir)
	}
	return dirs
}

// requireInsideProjectSkillRoot rejects every target outside the project skill roots.
func requireInsideProjectSkillRoot(workspaceRoot, target string) error {
	roots := projectSkillRoots(workspaceRoot)
	if len(roots) == 0 {
		return reject(RejectTargetOutsideProjectSkillRoots, "workspace root is not configured")
	}
	for _, root := range roots {
		rel, err := filepath.Rel(root, target)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return reject(RejectTargetOutsideProjectSkillRoots, "target %s is outside project skill roots %s", target, strings.Join(roots, ", "))
}

// requireNoSymlinkBelowRoot rejects a target when a path element below the
// workspace root is a symbolic link. Paths are compared as the caller spelled
// them, so a workspace root that is itself reached through a symlink still works.
func requireNoSymlinkBelowRoot(workspaceRoot, target string) error {
	base, err := filepath.Abs(filepath.Clean(workspaceRoot))
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target path: %w", err)
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return fmt.Errorf("compare target with workspace root: %w", err)
	}
	// shortcut: a target that spells the root through another path skips this
	// walk; requireInsideProjectSkillRoot still pins the resolved file. Upgrade
	// if callers start passing root paths that share spelling.
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	current := base
	for _, elem := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, elem)
		fi, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect target path %s: %w", current, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return reject(RejectTargetHasSymlink, "target path %s is a symbolic link", current)
		}
	}
	return nil
}

// requireRegularFile rejects a target that is not an existing regular file, so a
// proposal can never create a skill or write through a device, FIFO or directory.
func requireRegularFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect target skill: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return reject(RejectTargetNotRegularFile, "target %s is not a regular file", path)
	}
	return nil
}

// Seams so the unit test can assert fsync lands before the rename.
var (
	atomicWriteSync   = func(f *os.File) error { return f.Sync() }
	atomicWriteRename = os.Rename
)

// writeFileAtomic replaces path with data through a temp file in the same
// directory: write, flush, fsync, then rename. The target keeps its mode bits.
// Kept separate from the securefs permission helpers, which write in place.
func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".skill-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op once the rename below succeeded
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return fmt.Errorf("set temp file mode: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := atomicWriteSync(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := atomicWriteRename(tmp, path); err != nil {
		return fmt.Errorf("replace target skill: %w", err)
	}
	return nil
}

// resolvePathLenient resolves symlinks when it can and falls back to an absolute
// cleaned path; it is used for roots that may not exist yet.
func resolvePathLenient(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

// lockedTarget runs fn with this target locked against other appliers in this
// process and in other processes (M1-22, M1-23). Targets outside the project
// skill roots are passed straight through: requireKnownSkillPath refuses them
// with its own code, and no lock file is created for a path we will reject.
func (s *Store) lockedTarget(target string, fn func() (Proposal, error)) (Proposal, error) {
	if strings.TrimSpace(s.workspaceRoot) == "" || !strings.HasPrefix(strings.TrimSpace(target), "/") {
		return fn()
	}
	realRoot := resolvePathLenient(s.workspaceRoot)
	realTarget := resolvePathLenient(target)
	if !strings.HasPrefix(realTarget, realRoot+string(filepath.Separator)) {
		return fn()
	}
	unlock, err := s.lockTarget(s.targetLockKey(target))
	if err != nil {
		return Proposal{}, fmt.Errorf("lock target %s: %w", target, err)
	}
	defer unlock()
	return fn()
}

func findSkillCaseInsensitive(roots []string, skillName string) (*skills.Skill, bool) {
	if found, _, ok := skills.Find(roots, skillName); ok {
		return found, true
	}
	target := strings.EqualFold
	for _, skill := range skills.Discover(roots) {
		if skill != nil && target(skill.Name, skillName) {
			return skill, true
		}
	}
	return nil, false
}

func normalizedExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Fail closed: an unresolvable target is never written through the
		// caller's spelling (M1-04).
		return "", reject(RejectTargetOutsideProjectSkillRoots, "resolve target path %s: %v", abs, err)
	}
	return filepath.Clean(resolved), nil
}

func evidenceID(ev Evidence) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%s\n", ev.CreatedAt.Format(time.RFC3339Nano), ev.Kind, ev.SessionID, ev.Skill, ev.Summary)
	return "ev-" + hex.EncodeToString(h.Sum(nil))[:12]
}

func proposalID(p Proposal) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n%s\n", p.CreatedAt.Format(time.RFC3339Nano), p.Skill, p.SkillFilePath, p.Summary)
	return "sp-" + hex.EncodeToString(h.Sum(nil))[:12]
}

// proposedSkillBytes is the exact byte sequence an apply writes: the proposal
// text trimmed with one trailing newline. Both the report and the write use it
// so the hash a user approves is the hash that lands on disk (M1-52).
func proposedSkillBytes(p Proposal) []byte {
	return []byte(strings.TrimSpace(p.ProposedSkillMD) + "\n")
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func cleanSkillName(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func cleanToken(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, v)
	return strings.Trim(v, "-_")
}

func cleanID(v string) string {
	return cleanToken(v)
}

// secretPatterns are the credential shapes that would otherwise survive in a
// summary line: provider keys, bearer tokens and key=value secrets.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{8,}`),
	regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*\S+`),
}

// redactSecrets must run before truncate: a cut-off secret still leaks its
// prefix, so the text is scrubbed while it is whole (M1-29).
func redactSecrets(v string) string {
	for _, re := range secretPatterns {
		v = re.ReplaceAllString(v, "[redacted]")
	}
	return v
}

func truncate(v string, max int) string {
	if max <= 0 || len([]rune(v)) <= max {
		return v
	}
	r := []rune(v)
	return string(r[:max]) + "...[truncated]"
}

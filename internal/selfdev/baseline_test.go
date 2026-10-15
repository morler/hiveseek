package selfdev

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type baselineRecord struct {
	SchemaVersion         int      `json:"schema_version"`
	GateVersion           string   `json:"gate_version"`
	Status                string   `json:"status"`
	Approval              *string  `json:"approval"`
	Baseline              baseline `json:"baseline"`
	Measurement           measurement
	TimeoutsSeconds       map[string]float64    `json:"timeouts_seconds"`
	IdentityRequiredField []string              `json:"identity_required_fields"`
	Dependencies          dependencyDigest      `json:"dependencies"`
	TrustedFiles          trustedFileCollection `json:"trusted_files"`
}

type baseline struct {
	Commit      string `json:"commit"`
	ShortCommit string `json:"short_commit"`
	Tag         string `json:"tag"`
	Module      string `json:"module"`
}

type measurement struct {
	Commit          string             `json:"commit"`
	GoVersion       string             `json:"go_version"`
	Goos            string             `json:"goos"`
	Goarch          string             `json:"goarch"`
	Validation      []validationResult `json:"validation"`
	StartupSeconds  []float64          `json:"startup_ready_wall_seconds"`
	BinarySHA256    string             `json:"binary_sha256"`
	RealAPIKeyUsed  bool               `json:"real_api_key_used"`
	CandidateRanAny bool               `json:"candidate_executed"`
}

type validationResult struct {
	Argv        []string `json:"argv"`
	WallSeconds float64  `json:"wall_seconds"`
	ExitCode    int      `json:"exit_code"`
}

type dependencyDigest struct {
	Files      []digestEntry `json:"files"`
	SHA256     string        `json:"sha256"`
	HashFormat string        `json:"hash_format"`
}

type digestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type trustedFileCollection struct {
	Count      int           `json:"count"`
	SHA256     string        `json:"sha256"`
	HashFormat string        `json:"hash_format"`
	Files      []trustedFile `json:"files"`
}

type trustedFile struct {
	Path   string `json:"path"`
	Role   string `json:"role"`
	SHA256 string `json:"sha256"`
}

var (
	sha256Pattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	fullCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

const (
	wantTrustedFileCount = 337
	wantBaselineTests    = 281
	wantFixtures         = 21
	wantTrustedSupport   = 35
)

// TestM0BaselineRecord pins the structure and the measured headline numbers of
// docs/self-evolution/baseline-v1.json so that edits to the record stay
// consistent with the fixed constants and with the M0 report.
func TestM0BaselineRecord(t *testing.T) {
	rec := loadBaselineRecord(t)

	if rec.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", rec.SchemaVersion)
	}
	if rec.GateVersion != "m0-baseline-v1" {
		t.Fatalf("gate_version = %q, want m0-baseline-v1", rec.GateVersion)
	}
	if rec.Status != "proposed" {
		t.Fatalf("status = %q: the record stays proposed until a maintainer approves it", rec.Status)
	}
	if rec.Approval != nil {
		t.Fatalf("approval = %q, want null before maintainer approval", *rec.Approval)
	}

	if !fullCommitPattern.MatchString(rec.Baseline.Commit) {
		t.Fatalf("baseline.commit = %q, want a 40-character commit hash", rec.Baseline.Commit)
	}
	if !strings.HasPrefix(rec.Baseline.Commit, rec.Baseline.ShortCommit) {
		t.Fatalf("baseline.short_commit = %q is not a prefix of %q", rec.Baseline.ShortCommit, rec.Baseline.Commit)
	}
	if rec.Baseline.Tag != "v0.1.66" {
		t.Fatalf("baseline.tag = %q, want v0.1.66", rec.Baseline.Tag)
	}
	if rec.Baseline.Module != "github.com/usewhale/whale" {
		t.Fatalf("baseline.module = %q", rec.Baseline.Module)
	}

	if !fullCommitPattern.MatchString(rec.Measurement.Commit) {
		t.Fatalf("measurement.commit = %q, want a 40-character commit hash", rec.Measurement.Commit)
	}
	if !strings.Contains(rec.Measurement.GoVersion, "go1.") {
		t.Fatalf("measurement.go_version = %q, want the verbatim `go version` output", rec.Measurement.GoVersion)
	}
	if rec.Measurement.Goos == "" || rec.Measurement.Goarch == "" {
		t.Fatalf("measurement.goos/goarch must be recorded, got %q/%q", rec.Measurement.Goos, rec.Measurement.Goarch)
	}
	if !sha256Pattern.MatchString(rec.Measurement.BinarySHA256) {
		t.Fatalf("measurement.binary_sha256 = %q, want a sha256 hex digest", rec.Measurement.BinarySHA256)
	}
	if rec.Measurement.RealAPIKeyUsed {
		t.Fatalf("real_api_key_used = true: M0 measurements must not use a real API key")
	}
	if rec.Measurement.CandidateRanAny {
		t.Fatalf("candidate_executed = true: M0 forbids executing model-generated candidates")
	}
	if len(rec.Measurement.Validation) != 4 {
		t.Fatalf("validation entries = %d, want 4 (fmt-check, vet, test, build)", len(rec.Measurement.Validation))
	}
	for _, v := range rec.Measurement.Validation {
		if v.ExitCode != 0 {
			t.Fatalf("validation %v exited with %d, want 0", v.Argv, v.ExitCode)
		}
		if v.WallSeconds <= 0 {
			t.Fatalf("validation %v wall_seconds = %v, want a positive measurement", v.Argv, v.WallSeconds)
		}
	}
	if len(rec.Measurement.StartupSeconds) < 3 {
		t.Fatalf("startup samples = %d, want at least 3 individual values", len(rec.Measurement.StartupSeconds))
	}
	for _, s := range rec.Measurement.StartupSeconds {
		if s <= 0 {
			t.Fatalf("startup sample %v, want a positive measurement", s)
		}
	}

	wantTimeouts := map[string]float64{
		"verification": VerificationTimeout.Seconds(),
		"ready":        ReadyTimeout.Seconds(),
	}
	if len(rec.TimeoutsSeconds) != len(wantTimeouts) {
		t.Fatalf("timeouts_seconds has %d entries, want %d", len(rec.TimeoutsSeconds), len(wantTimeouts))
	}
	for name, want := range wantTimeouts {
		if got := rec.TimeoutsSeconds[name]; got != want {
			t.Fatalf("timeouts_seconds[%q] = %v, want %v (the constants in limits.go are the source of truth)", name, got, want)
		}
	}

	wantIdentity := []string{
		"source_root",
		"candidate_commit",
		"baseline_commit",
		"go_toolchain",
		"goos",
		"goarch",
		"build_args",
		"dependency_digest",
		"gate_version",
		"binary_sha256",
	}
	if !equalStrings(rec.IdentityRequiredField, wantIdentity) {
		t.Fatalf("identity_required_fields = %v, want %v", rec.IdentityRequiredField, wantIdentity)
	}

	checkDependencies(t, rec.Dependencies)
	checkTrustedFiles(t, rec.TrustedFiles)
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file to resolve the repository root")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func loadBaselineRecord(t *testing.T) baselineRecord {
	t.Helper()

	path := filepath.Join(repoRoot(t), "docs", "self-evolution", "baseline-v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read M0 baseline record: %v", err)
	}

	var rec baselineRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("parse M0 baseline record: %v", err)
	}
	return rec
}

func checkDependencies(t *testing.T, deps dependencyDigest) {
	t.Helper()

	if !sha256Pattern.MatchString(deps.SHA256) {
		t.Fatalf("dependencies.sha256 = %q, want a sha256 hex digest", deps.SHA256)
	}
	if !strings.Contains(deps.HashFormat, "path") || !strings.Contains(deps.HashFormat, "sha256") {
		t.Fatalf("dependencies.hash_format = %q, want the path/sha256 row format", deps.HashFormat)
	}
	var paths []string
	for _, f := range deps.Files {
		if f.Path == "" || !sha256Pattern.MatchString(f.SHA256) {
			t.Fatalf("dependency entry %+v, want a path and a sha256 digest", f)
		}
		paths = append(paths, f.Path)
	}
	if !equalStrings(paths, []string{"go.mod", "go.sum"}) {
		t.Fatalf("dependency files = %v, want [go.mod go.sum]", paths)
	}
	var rows strings.Builder
	for _, f := range deps.Files {
		fmt.Fprintf(&rows, "%s\t%s\n", f.Path, f.SHA256)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(rows.String()))); got != deps.SHA256 {
		t.Fatalf("dependencies.sha256 = %q, want %q recomputed from the listed rows", deps.SHA256, got)
	}
}

func checkTrustedFiles(t *testing.T, col trustedFileCollection) {
	t.Helper()

	if col.Count != wantTrustedFileCount {
		t.Fatalf("trusted_files.count = %d, want %d", col.Count, wantTrustedFileCount)
	}
	if len(col.Files) != col.Count {
		t.Fatalf("trusted_files.files = %d entries, want %d", len(col.Files), col.Count)
	}
	if !sha256Pattern.MatchString(col.SHA256) {
		t.Fatalf("trusted_files.sha256 = %q, want a sha256 hex digest", col.SHA256)
	}
	if !strings.Contains(col.HashFormat, "role") {
		t.Fatalf("trusted_files.hash_format = %q, want the role/path/sha256 row format", col.HashFormat)
	}

	roles := map[string]int{}
	previous := ""
	for i, f := range col.Files {
		if f.Path == "" || !sha256Pattern.MatchString(f.SHA256) {
			t.Fatalf("trusted file %+v, want a path and a sha256 digest", f)
		}
		if filepath.IsAbs(f.Path) || !filepath.IsLocal(filepath.ToSlash(f.Path)) {
			t.Fatalf("trusted file %q is not a repository-relative path", f.Path)
		}
		if i > 0 && f.Path <= previous {
			t.Fatalf("trusted file %q is not sorted after %q", f.Path, previous)
		}
		previous = f.Path

		switch f.Role {
		case "baseline_test":
			if !strings.HasSuffix(f.Path, "_test.go") {
				t.Fatalf("baseline_test %q is not a _test.go file", f.Path)
			}
		case "baseline_fixture":
			if !strings.Contains(f.Path, "testdata/") && !strings.Contains(f.Path, "fixtures/") {
				t.Fatalf("baseline_fixture %q is neither testdata nor fixture content", f.Path)
			}
		case "trusted_support":
			if strings.HasSuffix(f.Path, "_test.go") {
				t.Fatalf("trusted_support %q is a test file; it belongs to baseline_test", f.Path)
			}
		default:
			t.Fatalf("trusted file %q has unknown role %q", f.Path, f.Role)
		}
		roles[f.Role]++
	}
	var rows strings.Builder
	for _, f := range col.Files {
		fmt.Fprintf(&rows, "%s\t%s\t%s\n", f.Role, f.Path, f.SHA256)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(rows.String()))); got != col.SHA256 {
		t.Fatalf("trusted_files.sha256 = %q, want %q recomputed from the listed rows", col.SHA256, got)
	}

	want := map[string]int{
		"baseline_test":    wantBaselineTests,
		"baseline_fixture": wantFixtures,
		"trusted_support":  wantTrustedSupport,
	}
	for role, wantCount := range want {
		if roles[role] != wantCount {
			t.Fatalf("trusted file role %q has %d entries, want %d", role, roles[role], wantCount)
		}
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestM0BaselineRecordMatchesMeasurementCommit re-hashes every trusted file and
// dependency from the tree at measurement.commit. Later commits may legitimately
// change the working tree (M1 edits baseline tests), so this check reads git
// history and skips when git or that commit is unavailable.
func TestM0BaselineRecordMatchesMeasurementCommit(t *testing.T) {
	rec := loadBaselineRecord(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable; cannot verify the record against its measurement commit")
	}
	root := repoRoot(t)
	commit := rec.Measurement.Commit
	if err := exec.Command("git", "-C", root, "cat-file", "-e", commit+"^{commit}").Run(); err != nil {
		t.Skipf("measurement commit %s is not present in this checkout", commit)
	}

	type hashTarget struct{ path, sha256 string }
	targets := make([]hashTarget, 0, len(rec.TrustedFiles.Files)+len(rec.Dependencies.Files))
	for _, f := range rec.TrustedFiles.Files {
		targets = append(targets, hashTarget{f.Path, f.SHA256})
	}
	for _, f := range rec.Dependencies.Files {
		targets = append(targets, hashTarget{f.Path, f.SHA256})
	}

	cmd := exec.Command("git", "-C", root, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("open git cat-file stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("open git cat-file stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start git cat-file: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})

	out := bufio.NewReader(stdout)
	var bad int
	var samples []string
	for _, target := range targets {
		if _, err := fmt.Fprintf(stdin, "%s:%s\n", commit, target.path); err != nil {
			t.Fatalf("request %s: %v", target.path, err)
		}
		header, err := out.ReadString('\n')
		if err != nil {
			t.Fatalf("read blob header for %s: %v", target.path, err)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[1] != "blob" {
			t.Fatalf("git cat-file %s:%s returned %q, want a blob", commit, target.path, strings.TrimSpace(header))
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("parse blob size for %s: %v", target.path, err)
		}
		content := make([]byte, size)
		if _, err := io.ReadFull(out, content); err != nil {
			t.Fatalf("read blob for %s: %v", target.path, err)
		}
		if _, err := out.ReadByte(); err != nil {
			t.Fatalf("read blob terminator for %s: %v", target.path, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != target.sha256 {
			bad++
			if len(samples) < 5 {
				samples = append(samples, fmt.Sprintf("%s: %s at %s, record says %s", target.path, got[:12], commit[:12], target.sha256[:12]))
			}
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d recorded files do not match %s:\n%s", bad, len(targets), commit, strings.Join(samples, "\n"))
	}
}

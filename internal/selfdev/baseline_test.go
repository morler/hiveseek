package selfdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
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
	if want := (15 * time.Minute).Seconds(); wantTimeouts["verification"] != want {
		t.Fatalf("VerificationTimeout = %v, want %v", VerificationTimeout, 15*time.Minute)
	}
	if want := (30 * time.Second).Seconds(); wantTimeouts["ready"] != want {
		t.Fatalf("ReadyTimeout = %v, want %v", ReadyTimeout, 30*time.Second)
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

func loadBaselineRecord(t *testing.T) baselineRecord {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file to resolve the repository root")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "docs", "self-evolution", "baseline-v1.json")
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

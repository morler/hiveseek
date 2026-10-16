package skillsimprover

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestConcurrentApplyAdmitsOneProposalPerTarget checks the in-process
// serialization: two proposals for the same file are started together, and
// only the one that reaches the target first may pass the SHA check (M1-22).
func TestConcurrentApplyAdmitsOneProposalPerTarget(t *testing.T) {
	workspace := t.TempDir()
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(filepath.Join(t.TempDir(), "skills-improver"), workspace)
	orig, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	bodies := []string{"first body", "second body"}
	ids := make([]string, 0, len(bodies))
	for _, body := range bodies {
		p, err := store.SaveProposal(Proposal{
			Skill:           "demo",
			SkillFilePath:   skillPath,
			OriginalSHA256:  sha256Hex(orig),
			Summary:         "change to " + body,
			ProposedSkillMD: skillMarkdown("demo", body),
		})
		if err != nil {
			t.Fatalf("save proposal: %v", err)
		}
		ids = append(ids, p.ID)
	}

	const attempts = 8
	start := make(chan struct{})
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, results[i] = store.ApplyProposal(ids[i%len(ids)])
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d applies succeeded, want exactly one: %v", wins, results)
	}
	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, body := range bodies {
		if string(got) == skillMarkdown("demo", body) {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("target holds neither proposal, got %q", string(got))
	}
}

// TestCrossProcessApplyLockSerializes holds the target lock in this process and
// uses a second process to prove an apply waits for it instead of writing
// alongside it (M1-23). The child re-runs this test with SKILLSIMPROVER_LOCK_CHILD=1.
func TestCrossProcessApplyLockSerializes(t *testing.T) {
	if os.Getenv("SKILLSIMPROVER_LOCK_CHILD") == "1" {
		store := NewStore(os.Getenv("SKILLSIMPROVER_CHILD_DATA"), os.Getenv("SKILLSIMPROVER_CHILD_WORKSPACE"))
		if _, err := store.ApplyProposal(os.Getenv("SKILLSIMPROVER_CHILD_PROPOSAL")); err != nil {
			t.Fatalf("child apply: %v", err)
		}
		return
	}

	workspace := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "skills-improver")
	skillPath := writeTestSkill(t, workspace, "demo", "old body")
	store := NewStore(dataDir, workspace)
	orig, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	proposed := skillMarkdown("demo", "new body")
	p, err := store.SaveProposal(Proposal{
		Skill:           "demo",
		SkillFilePath:   skillPath,
		OriginalSHA256:  sha256Hex(orig),
		Summary:         "change",
		ProposedSkillMD: proposed,
	})
	if err != nil {
		t.Fatalf("save proposal: %v", err)
	}

	unlock, err := store.lockTarget(store.targetLockKey(skillPath))
	if err != nil {
		t.Fatalf("lock target: %v", err)
	}
	release := sync.OnceFunc(unlock)
	defer release()

	cmd := exec.Command(os.Args[0], "-test.run", "^TestCrossProcessApplyLockSerializes$")
	cmd.Env = append(os.Environ(),
		"SKILLSIMPROVER_LOCK_CHILD=1",
		"SKILLSIMPROVER_CHILD_DATA="+dataDir,
		"SKILLSIMPROVER_CHILD_WORKSPACE="+workspace,
		"SKILLSIMPROVER_CHILD_PROPOSAL="+p.ID,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		release()
		if err == nil {
			t.Fatal("a second process applied while the target lock was held")
		}
		t.Fatalf("the child process did not wait for the lock: %v\n%s", err, out.String())
	case <-time.After(time.Second):
	}
	if got, err := os.ReadFile(skillPath); err != nil || string(got) != string(orig) {
		release()
		t.Fatalf("the target changed while the lock was held: %q (err=%v)", string(got), err)
	}

	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("child apply failed after the lock was released: %v\n%s", err, out.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the child did not finish after the lock was released")
	}
	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != proposed {
		t.Fatalf("target = %q, want the single applied change %q", string(got), proposed)
	}
	if strings.Count(string(got), "new body") != 1 {
		t.Fatalf("the change must be written once, got %q", string(got))
	}
}

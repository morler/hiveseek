package delmresearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is relative to this package directory, bench/delmresearch.
const repoRoot = "../.."

func TestTasksAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, task := range Tasks {
		if task.ID == "" || task.Prompt == "" {
			t.Errorf("task %q: ID and Prompt are required", task.ID)
		}
		if seen[task.ID] {
			t.Errorf("duplicate task ID %q", task.ID)
		}
		seen[task.ID] = true
		if len(task.Facts) < 3 {
			t.Errorf("task %s: want at least 3 facts for a multi-step question, got %d", task.ID, len(task.Facts))
		}
		for _, f := range task.Facts {
			if f.Expect == "" || f.File == "" || f.Anchor == "" {
				t.Errorf("task %s: fact %+v has an empty field", task.ID, f)
			}
		}
	}
}

func TestPromptsDoNotLeakAnswers(t *testing.T) {
	for _, task := range Tasks {
		prompt := strings.ToLower(task.Prompt)
		for _, f := range task.Facts {
			if strings.Contains(prompt, strings.ToLower(f.Expect)) {
				t.Errorf("task %s: prompt contains answer %q", task.ID, f.Expect)
			}
		}
	}
}

// TestAnswerKeyAnchors fails when a cited file no longer contains its anchor,
// which means the answer key has drifted from the pinned commit.
func TestAnswerKeyAnchors(t *testing.T) {
	bodies := map[string]string{}
	for _, task := range Tasks {
		for _, f := range task.Facts {
			body, ok := bodies[f.File]
			if !ok {
				b, err := os.ReadFile(filepath.Join(repoRoot, f.File))
				if err != nil {
					t.Fatalf("task %s: %v", task.ID, err)
				}
				body = string(b)
				bodies[f.File] = body
			}
			if !strings.Contains(body, f.Anchor) {
				t.Errorf("task %s: anchor %q not found in %s; answer key has drifted", task.ID, f.Anchor, f.File)
			}
		}
	}
}

func TestScoreAndPass(t *testing.T) {
	task := Task{ID: "x", Facts: []Fact{{Expect: "Alpha"}, {Expect: "beta"}, {Expect: "gamma"}}}
	tests := []struct {
		name   string
		answer string
		hit    int
		pass   bool
	}{
		{"all facts, mixed case", "ALPHA, Beta and gamma", 3, true},
		{"one fact missing", "alpha and beta", 2, false},
		{"empty answer", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, total := task.Score(tt.answer)
			if hit != tt.hit || total != 3 {
				t.Errorf("Score = (%d, %d), want (%d, 3)", hit, total, tt.hit)
			}
			if got := task.Pass(tt.answer); got != tt.pass {
				t.Errorf("Pass = %v, want %v", got, tt.pass)
			}
		})
	}
}

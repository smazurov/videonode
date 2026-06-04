package envctl

import "testing"

func TestWorktreeContains(t *testing.T) {
	const wt = "/home/u/dev/repo/.claude/worktrees/feat-a"
	tests := []struct {
		name  string
		owner string
		dir   string
		want  bool
	}{
		{"exact root", wt, wt, true},
		{"subdir", wt, wt + "/ui/src", true},
		{"sibling prefix is not a match", wt, wt + "-2", false},
		{"unrelated worktree", wt, "/home/u/dev/repo/.claude/worktrees/feat-b", false},
		{"parent is not a match", wt, "/home/u/dev/repo", false},
		{"empty owner", "", wt, false},
		{"empty dir", wt, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worktreeContains(tt.owner, tt.dir); got != tt.want {
				t.Errorf("worktreeContains(%q, %q) = %v, want %v", tt.owner, tt.dir, got, tt.want)
			}
		})
	}
}

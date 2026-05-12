package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLocalGitIdentityUsesGlobalScope(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("creating repo dir: %v", err)
	}

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}

	runGit(homeDir, "config", "--global", "user.name", "Global User")
	runGit(homeDir, "config", "--global", "user.email", "global@example.com")
	runGit(repoDir, "init")
	runGit(repoDir, "config", "user.name", "Local User")
	runGit(repoDir, "config", "user.email", "local@example.com")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("changing cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	gotName, gotEmail := readLocalGitIdentity()
	if gotName != "Global User" {
		t.Fatalf("git user name = %q, want %q", gotName, "Global User")
	}
	if gotEmail != "global@example.com" {
		t.Fatalf("git user email = %q, want %q", gotEmail, "global@example.com")
	}
}

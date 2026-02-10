package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "test")
	run("config", "user.email", "test@test")
	// Initial commit.
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "initial")
	return dir
}

func TestDiffNameOnly(t *testing.T) {
	dir := initTestRepo(t)

	baseSHA, err := HeadSHA(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new file and commit it.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "add new file")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	files, err := DiffNameOnly(dir, baseSHA)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 || files[0] != "new.txt" {
		t.Errorf("DiffNameOnly = %v, want [new.txt]", files)
	}
}

func TestDiffNameOnly_NoChanges(t *testing.T) {
	dir := initTestRepo(t)

	sha, err := HeadSHA(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := DiffNameOnly(dir, sha)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("DiffNameOnly with no changes = %v, want []", files)
	}
}

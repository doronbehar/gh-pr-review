package gitctx

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// requireGit skips a test that cannot run without the git binary, such as in
// a sandboxed build.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

// newRepo creates a repository with one commit and makes it the working
// directory for the test.
func newRepo(t *testing.T) {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	t.Chdir(dir)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(cmd.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
	}

	run("init", "--initial-branch=main", ".")
	run("commit", "--allow-empty", "-m", "initial")
}

func TestCurrentBranchReportsTheCheckedOutBranch(t *testing.T) {
	newRepo(t)

	branch, err := CurrentBranch()
	if err != nil {
		t.Fatalf("current branch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
}

// A local branch can track a remote branch under another name, and the pull
// request is keyed by the remote one.
func TestCurrentBranchPrefersTheTrackedRefName(t *testing.T) {
	newRepo(t)

	cmd := exec.Command("git", "config", "branch.main.merge", "refs/heads/release")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure branch.main.merge: %v: %s", err, output)
	}

	branch, err := CurrentBranch()
	if err != nil {
		t.Fatalf("current branch: %v", err)
	}
	if branch != "release" {
		t.Fatalf("expected release, got %q", branch)
	}
}

func TestCurrentBranchOnDetachedHead(t *testing.T) {
	newRepo(t)

	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("detach HEAD: %v: %s", err, output)
	}

	_, err := CurrentBranch()
	if !errors.Is(err, ErrNoBranch) {
		t.Fatalf("expected ErrNoBranch, got %v", err)
	}
}

func TestCurrentBranchOutsideARepository(t *testing.T) {
	requireGit(t)
	t.Chdir(t.TempDir())

	_, err := CurrentBranch()
	if err == nil {
		t.Fatal("expected an error outside a repository")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("expected git's own complaint, got %v", err)
	}
}

// GH_REPO overrides the remotes, which is also the one repository lookup that
// needs neither git state nor credentials.
func TestCurrentRepoHonorsGHRepo(t *testing.T) {
	t.Setenv("GH_REPO", "octo/demo")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	repo, err := CurrentRepo()
	if err != nil {
		t.Fatalf("current repo: %v", err)
	}
	if repo.Owner != "octo" || repo.Name != "demo" {
		t.Fatalf("unexpected repository: %#v", repo)
	}
	if repo.Slug() != "octo/demo" {
		t.Fatalf("unexpected slug: %q", repo.Slug())
	}
}

func TestCurrentRepoWithoutRemotes(t *testing.T) {
	t.Setenv("GH_REPO", "")
	newRepo(t)

	if _, err := CurrentRepo(); err == nil {
		t.Fatal("expected an error without remotes")
	}
}

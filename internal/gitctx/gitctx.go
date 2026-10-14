// Package gitctx reads the local git context: the GitHub repository that the
// working directory tracks, and the branch that is checked out.
package gitctx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// ErrNoBranch reports that HEAD does not point at a branch, which happens on a
// detached HEAD or in a repository without commits.
var ErrNoBranch = errors.New("HEAD is not on a branch")

// errSilentExit marks a git command that ran and failed without writing
// anything, which for the lookups here means "no such setting".
var errSilentExit = errors.New("git exited without output")

// Repo identifies the GitHub repository that the working directory tracks.
type Repo struct {
	Owner string
	Name  string
	Host  string
}

// Slug renders the repository as "owner/name".
func (r Repo) Slug() string {
	return r.Owner + "/" + r.Name
}

// CurrentRepo reports the repository the working directory tracks, by the same
// rules gh itself applies: the GH_REPO override, then the git remotes, with ssh
// aliases translated, remotes filtered to the hosts gh is logged in to,
// `gh repo set-default` honored, and `upstream` preferred over `origin`. It
// reads local git and gh configuration only; no API requests are made.
func CurrentRepo() (Repo, error) {
	current, err := repository.Current()
	if err != nil {
		// Remotes are matched against the hosts gh knows, so with no
		// credentials at all every remote looks foreign. Say that instead,
		// but only once the lookup has had its chance: GH_REPO answers
		// without needing a host, or any git state.
		if len(auth.KnownHosts()) == 0 {
			return Repo{}, errors.New("no authenticated GitHub host: run `gh auth login`, or set GH_TOKEN")
		}
		return Repo{}, &flatError{err: err}
	}
	if current.Owner == "" || current.Name == "" {
		return Repo{}, fmt.Errorf("git remotes resolved to an incomplete repository %q", current.Owner+"/"+current.Name)
	}
	return Repo{Owner: current.Owner, Name: current.Name, Host: current.Host}, nil
}

// CurrentBranch reports the head ref name that the checked out branch pushes
// to. That is branch.<name>.merge when configured, since a local branch can
// track a remote branch under another name, and the local name otherwise.
func CurrentBranch() (string, error) {
	head, err := git("symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		if errors.Is(err, errSilentExit) {
			return "", ErrNoBranch
		}
		return "", err
	}

	local := strings.TrimPrefix(head, "refs/heads/")
	if local == "" {
		return "", ErrNoBranch
	}

	// An unset branch.<name>.merge exits non-zero with no output; the branch
	// then pushes under its own name.
	merge, err := git("config", "--get", "branch."+local+".merge")
	if err != nil && !errors.Is(err, errSilentExit) {
		return "", err
	}
	if tracked := strings.TrimPrefix(merge, "refs/heads/"); tracked != "" {
		return tracked, nil
	}

	return local, nil
}

// flatError keeps an error on one line. The git failures reported through
// go-gh carry git's own multi-line output, which would otherwise break up a
// command's single-line error.
type flatError struct {
	err error
}

func (e *flatError) Error() string {
	return strings.Join(strings.Fields(e.err.Error()), " ")
}

func (e *flatError) Unwrap() error {
	return e.err
}

func git(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if message := strings.TrimSpace(stderr.String()); message != "" {
				return "", fmt.Errorf("git %s: %s", args[0], message)
			}
			return "", errSilentExit
		}
		return "", fmt.Errorf("run git %s: %w", args[0], err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

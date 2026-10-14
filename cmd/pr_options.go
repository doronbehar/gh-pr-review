package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agynio/gh-pr-review/internal/pulls"
	"github.com/agynio/gh-pr-review/internal/resolver"
)

// prOptions carries the flags that address a pull request. One instance is
// bound on the root command, so every command shares a single definition of
// these flags.
type prOptions struct {
	Repo     string
	Pull     int
	Selector string
}

// bind registers the shared flags as persistent flags so every command below
// cmd inherits them.
func (o *prOptions) bind(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&o.Repo, "repo", "R", "", "Repository in 'owner/repo' format")
	cmd.PersistentFlags().IntVar(&o.Pull, "pr", 0, "Pull request number")
}

// identity resolves the pull request addressed by the shared flags and the
// optional positional selector, inferring from the git context whatever was
// not given.
func (o *prOptions) identity(args []string) (resolver.Identity, error) {
	if len(args) > 0 {
		o.Selector = args[0]
	}

	host, err := o.infer(strings.TrimSpace(os.Getenv("GH_HOST")))
	if err != nil {
		return resolver.Identity{}, err
	}

	selector, err := resolver.NormalizeSelector(o.Selector, o.Pull)
	if err != nil {
		return resolver.Identity{}, err
	}

	return resolver.Resolve(selector, o.Repo, host)
}

// infer fills in the repository and pull request from the git context when the
// flags leave them unset, and reports the host to resolve against: the
// inferred remote's host unless GH_HOST names one.
func (o *prOptions) infer(host string) (string, error) {
	// A pull request URL carries the host, repository, and number already.
	if resolver.IsPullURL(o.Selector) {
		return host, nil
	}

	repoGiven := o.Repo != ""
	if !repoGiven {
		repo, err := currentRepo()
		if err != nil {
			return host, fmt.Errorf("no --repo given and none could be inferred from the git remotes: %w", err)
		}
		o.Repo = repo.Slug()
		if host == "" {
			host = repo.Host
		}
	}

	// Only infer the pull request alongside the repository: the checked out
	// branch says nothing about a repository named by --repo.
	if o.Selector == "" && o.Pull == 0 && !repoGiven {
		number, err := o.inferPull(host)
		if err != nil {
			return host, err
		}
		o.Pull = number
	}

	return host, nil
}

// inferPull looks up the open pull request that the checked out branch heads.
func (o *prOptions) inferPull(host string) (int, error) {
	branch, err := currentBranch()
	if err != nil {
		return 0, fmt.Errorf("no pull request given and the current branch could not be determined: %w", err)
	}

	owner, name, found := strings.Cut(o.Repo, "/")
	if !found {
		return 0, fmt.Errorf("no pull request given and repository %q is not in owner/repo form", o.Repo)
	}

	number, err := pulls.FindByBranch(apiClientFactory(host), owner, name, branch)
	if err != nil {
		return 0, fmt.Errorf("no pull request given and none could be inferred: %w", err)
	}
	return number, nil
}

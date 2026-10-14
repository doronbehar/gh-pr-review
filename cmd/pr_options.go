package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/agynio/gh-pr-review/internal/resolver"
)

// prOptions carries the flags that address a pull request. Every command group
// binds one instance on its parent command so its subcommands share a single
// definition of these flags.
type prOptions struct {
	Repo     string
	Pull     int
	Selector string
}

// bind registers the shared flags as persistent flags so all subcommands of
// cmd inherit them.
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

	if err := o.infer(); err != nil {
		return resolver.Identity{}, err
	}

	selector, err := resolver.NormalizeSelector(o.Selector, o.Pull)
	if err != nil {
		return resolver.Identity{}, err
	}

	return resolver.Resolve(selector, o.Repo, os.Getenv("GH_HOST"))
}

// infer fills in the repository and pull request from the current directory
// and branch when the flags leave them unset.
func (o *prOptions) infer() error {
	// A pull request URL carries the host, repository, and number already.
	if resolver.IsPullURL(o.Selector) {
		return nil
	}

	repoGiven := o.Repo != ""
	if !repoGiven {
		repo, err := currentRepo()
		if err != nil {
			return fmt.Errorf("no --repo given and none could be inferred from the current directory: %w", err)
		}
		o.Repo = repo
	}

	// Only infer the pull request alongside the repository: the checked out
	// branch says nothing about a repository named by --repo.
	if o.Selector == "" && o.Pull == 0 && !repoGiven {
		number, err := currentPR()
		if err != nil {
			return fmt.Errorf("no pull request given and none could be inferred from the current branch: %w", err)
		}
		o.Pull = number
	}

	return nil
}

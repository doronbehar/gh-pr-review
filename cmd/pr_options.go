package cmd

import (
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
// optional positional selector.
func (o *prOptions) identity(args []string) (resolver.Identity, error) {
	if len(args) > 0 {
		o.Selector = args[0]
	}

	selector, err := resolver.NormalizeSelector(o.Selector, o.Pull)
	if err != nil {
		return resolver.Identity{}, err
	}

	return resolver.Resolve(selector, o.Repo, os.Getenv("GH_HOST"))
}

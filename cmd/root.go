package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Execute sets up the root command tree and executes it.
func Execute() error {
	root := newRootCommand()
	return root.Execute()
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "gh-pr-review",
		Short:         "PR review helper commands for gh",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Every command addresses a pull request, so the flags that name one are
	// defined once here and inherited throughout.
	opts := &prOptions{}
	opts.bind(cmd)

	cmd.AddCommand(newReplyCommand(opts))
	cmd.AddCommand(newStartCommand(opts))
	cmd.AddCommand(newCommentCommand(opts))
	cmd.AddCommand(newSubmitCommand(opts))
	cmd.AddCommand(newViewCommand(opts))
	cmd.AddCommand(newThreadsCommand(opts))

	return cmd
}

// ExecuteOrExit runs the command tree and exits with a non-zero status on error.
func ExecuteOrExit() {
	if err := Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agynio/gh-pr-review/internal/resolver"
	"github.com/agynio/gh-pr-review/internal/threads"
)

func newThreadsCommand(shared *prOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "threads",
		Short: "Inspect and resolve pull request review threads",
	}

	cmd.AddCommand(newThreadsListCommand(shared))
	cmd.AddCommand(newThreadsResolveCommand(shared))
	cmd.AddCommand(newThreadsUnresolveCommand(shared))

	return cmd
}

func newThreadsListCommand(shared *prOptions) *cobra.Command {
	opts := &threadsListOptions{}

	cmd := &cobra.Command{
		Use:   "list [<number> | <url>]",
		Short: "List review threads for a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identity, err := shared.identity(args)
			if err != nil {
				return err
			}
			return runThreadsList(cmd, identity, opts)
		},
	}

	cmd.Flags().BoolVar(&opts.UnresolvedOnly, "unresolved", false, "Filter to unresolved threads only")
	cmd.Flags().BoolVar(&opts.MineOnly, "mine", false, "Show only threads involving or resolvable by the viewer")

	return cmd
}

type threadsListOptions struct {
	UnresolvedOnly bool
	MineOnly       bool
}

func runThreadsList(cmd *cobra.Command, identity resolver.Identity, opts *threadsListOptions) error {
	service := threads.NewService(apiClientFactory(identity.Host))
	payload, err := service.List(identity, threads.ListOptions{
		OnlyUnresolved: opts.UnresolvedOnly,
		MineOnly:       opts.MineOnly,
	})
	if err != nil {
		return err
	}

	return encodeJSON(cmd, payload)
}

func newThreadsResolveCommand(shared *prOptions) *cobra.Command {
	return newThreadsMutationCommand(shared, true)
}

func newThreadsUnresolveCommand(shared *prOptions) *cobra.Command {
	return newThreadsMutationCommand(shared, false)
}

func newThreadsMutationCommand(shared *prOptions, resolve bool) *cobra.Command {
	opts := &threadsMutationOptions{}

	use := "resolve"
	short := "Resolve a review thread"
	if !resolve {
		use = "unresolve"
		short = "Reopen a review thread"
	}

	cmd := &cobra.Command{
		Use:   use + " [<number> | <url>]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.Validate(); err != nil {
				return err
			}
			identity, err := shared.identity(args)
			if err != nil {
				return err
			}
			return runThreadsMutation(cmd, identity, opts, resolve)
		},
	}

	cmd.Flags().StringVar(&opts.ThreadID, "thread-id", "", "GraphQL node ID for the review thread")

	return cmd
}

type threadsMutationOptions struct {
	ThreadID string
}

func (o *threadsMutationOptions) Validate() error {
	if strings.TrimSpace(o.ThreadID) == "" {
		return errors.New("--thread-id is required")
	}
	return nil
}

func runThreadsMutation(cmd *cobra.Command, identity resolver.Identity, opts *threadsMutationOptions, resolve bool) error {
	service := threads.NewService(apiClientFactory(identity.Host))
	action := threads.ActionOptions{ThreadID: strings.TrimSpace(opts.ThreadID)}

	var result threads.ActionResult
	var err error
	if resolve {
		result, err = service.Resolve(identity, action)
	} else {
		result, err = service.Unresolve(identity, action)
	}
	if err != nil {
		return err
	}
	return encodeJSON(cmd, result)
}

package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agynio/gh-pr-review/internal/report"
)

// newReviewViewCommand builds the canonical `review view` command.
func newReviewViewCommand(shared *prOptions) *cobra.Command {
	cmd := newViewCommand(shared)
	cmd.Long = `View a structured review summary (GraphQL).

Also available as 'gh pr-review comments view'.`
	return cmd
}

// newCommentsViewCommand exposes the same command as `comments view`, so the
// thread ids it reports can be looked up from the command group that consumes
// them. Cobra aliases cannot span parents, so this is a second instance of the
// same command rather than an entry in its Aliases.
func newCommentsViewCommand(shared *prOptions) *cobra.Command {
	cmd := newViewCommand(shared)
	cmd.Short = "Alias for 'review view': view a structured review summary (GraphQL)"
	cmd.Long = `Alias for 'gh pr-review review view'.

Views a structured review summary (GraphQL): reviews, their inline comment
threads, and the thread ids that 'comments reply' takes.`
	return cmd
}

func newViewCommand(shared *prOptions) *cobra.Command {
	opts := &reviewViewOptions{}

	cmd := &cobra.Command{
		Use:   "view [<number> | <url>]",
		Short: "View a structured review summary (GraphQL)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewView(cmd, shared, opts, args)
		},
	}

	cmd.Flags().StringVar(&opts.Reviewer, "reviewer", "", "Filter to a specific reviewer (login)")
	cmd.Flags().StringSliceVar(&opts.States, "states", nil, "Comma-separated review states (APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED)")
	cmd.Flags().BoolVar(&opts.Unresolved, "unresolved", false, "Only include unresolved threads")
	cmd.Flags().BoolVar(&opts.NotOutdated, "not_outdated", false, "Exclude outdated threads")
	cmd.Flags().IntVar(&opts.TailReplies, "tail", 0, "Limit to the last N replies per thread (0 = all)")
	cmd.Flags().BoolVar(&opts.IncludeCommentNodeID, "include-comment-node-id", false, "Include comment_node_id fields for parent comments and replies")

	return cmd
}

type reviewViewOptions struct {
	Reviewer             string
	States               []string
	Unresolved           bool
	NotOutdated          bool
	TailReplies          int
	IncludeCommentNodeID bool
}

func runReviewView(cmd *cobra.Command, shared *prOptions, opts *reviewViewOptions, args []string) error {
	if opts.TailReplies < 0 {
		return fmt.Errorf("invalid --tail value %d: must be non-negative", opts.TailReplies)
	}

	states, statesProvided, err := parseStateFilters(opts.States)
	if err != nil {
		return err
	}

	identity, err := shared.identity(args)
	if err != nil {
		return err
	}

	service := report.NewService(apiClientFactory(identity.Host))
	output, err := service.Fetch(identity, report.Options{
		Reviewer:             strings.TrimSpace(opts.Reviewer),
		States:               states,
		StatesProvided:       statesProvided,
		RequireUnresolved:    opts.Unresolved,
		RequireNotOutdated:   opts.NotOutdated,
		TailReplies:          opts.TailReplies,
		IncludeCommentNodeID: opts.IncludeCommentNodeID,
	})
	if err != nil {
		return err
	}

	return encodeJSON(cmd, output)
}

func parseStateFilters(raw []string) ([]report.State, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}

	valid := map[string]report.State{
		"APPROVED":          report.StateApproved,
		"CHANGES_REQUESTED": report.StateChangesRequested,
		"COMMENTED":         report.StateCommented,
		"DISMISSED":         report.StateDismissed,
	}
	allowed := make([]string, 0, len(valid))
	for key := range valid {
		allowed = append(allowed, key)
	}
	sort.Strings(allowed)

	temp := make(map[report.State]struct{})
	states := make([]report.State, 0, len(raw))
	for _, entry := range raw {
		parts := strings.Split(entry, ",")
		for _, part := range parts {
			candidate := strings.ToUpper(strings.TrimSpace(part))
			if candidate == "" {
				continue
			}
			state, ok := valid[candidate]
			if !ok {
				return nil, false, fmt.Errorf("invalid review state %q (allowed: %s)", part, strings.Join(allowed, ", "))
			}
			if _, seen := temp[state]; seen {
				continue
			}
			temp[state] = struct{}{}
			states = append(states, state)
		}
	}

	if len(states) == 0 {
		return nil, false, fmt.Errorf("no valid states provided")
	}

	return states, true, nil
}

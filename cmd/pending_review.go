package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	reviewsvc "github.com/agynio/gh-pr-review/internal/review"
)

// pendingReviewOptions carries the flags shared by the subcommands that act on
// an existing pending review.
type pendingReviewOptions struct {
	ReviewID string
	Body     string
}

func (o *pendingReviewOptions) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.ReviewID, "review-id", "", "Review identifier (GraphQL review node ID)")
	cmd.Flags().StringVar(&o.Body, "body", "", "Comment or review body")
}

func newStartCommand(shared *prOptions) *cobra.Command {
	var commit string

	cmd := &cobra.Command{
		Use:   "start [<number> | <url>]",
		Short: "Open a pending review",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identity, err := shared.identity(args)
			if err != nil {
				return err
			}
			service := reviewsvc.NewService(apiClientFactory(identity.Host))
			state, err := service.Start(identity, strings.TrimSpace(commit))
			if err != nil {
				return err
			}
			return encodeJSON(cmd, state)
		},
	}

	cmd.Flags().StringVar(&commit, "commit", "", "Commit SHA for review start (defaults to current head)")

	return cmd
}

func newCommentCommand(shared *prOptions) *cobra.Command {
	pending := &pendingReviewOptions{}
	opts := &reviewCommentOptions{Side: "RIGHT"}

	cmd := &cobra.Command{
		Use:   "comment [<number> | <url>]",
		Short: "Start a new inline comment thread on a pending review",
		Long: `Start a new inline comment thread on a pending review.

Each invocation creates a fresh thread at --path/--line; it cannot post into an
existing thread. To reply to a thread, use 'gh pr-review reply' with the
thread id from 'gh pr-review view'.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, err := buildThreadInput(pending, opts)
			if err != nil {
				return err
			}
			identity, err := shared.identity(args)
			if err != nil {
				return err
			}
			service := reviewsvc.NewService(apiClientFactory(identity.Host))
			thread, err := service.AddThread(identity, input)
			if err != nil {
				return err
			}
			return encodeJSON(cmd, thread)
		},
	}

	pending.bind(cmd)
	cmd.Flags().StringVar(&opts.Path, "path", "", "File path for inline comment")
	cmd.Flags().IntVar(&opts.Line, "line", 0, "Line number for inline comment")
	cmd.Flags().StringVar(&opts.Side, "side", opts.Side, "Diff side for inline comment (LEFT or RIGHT)")
	cmd.Flags().IntVar(&opts.StartLine, "start-line", 0, "Start line for multi-line comments")
	cmd.Flags().StringVar(&opts.StartSide, "start-side", "", "Start side for multi-line comments")

	return cmd
}

type reviewCommentOptions struct {
	Path      string
	Line      int
	Side      string
	StartLine int
	StartSide string
}

func buildThreadInput(pending *pendingReviewOptions, opts *reviewCommentOptions) (reviewsvc.ThreadInput, error) {
	reviewID := strings.TrimSpace(pending.ReviewID)
	if reviewID == "" {
		return reviewsvc.ThreadInput{}, errors.New("--review-id is required")
	}
	if !strings.HasPrefix(reviewID, "PRR_") {
		return reviewsvc.ThreadInput{}, fmt.Errorf("invalid --review-id %q: must be a GraphQL node id (PRR_...)", pending.ReviewID)
	}

	side, err := normalizeSide(opts.Side)
	if err != nil {
		return reviewsvc.ThreadInput{}, err
	}
	var startLine *int
	if opts.StartLine > 0 {
		startLine = &opts.StartLine
	}
	var startSide *string
	if opts.StartSide != "" {
		normalized, err := normalizeSide(opts.StartSide)
		if err != nil {
			return reviewsvc.ThreadInput{}, fmt.Errorf("invalid start-side: %w", err)
		}
		startSide = &normalized
	}

	return reviewsvc.ThreadInput{
		ReviewID:  reviewID,
		Path:      strings.TrimSpace(opts.Path),
		Line:      opts.Line,
		Side:      side,
		StartLine: startLine,
		StartSide: startSide,
		Body:      pending.Body,
	}, nil
}

func newSubmitCommand(shared *prOptions) *cobra.Command {
	pending := &pendingReviewOptions{}
	event := "COMMENT"

	cmd := &cobra.Command{
		Use:   "submit [<number> | <url>]",
		Short: "Submit a pending review",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedEvent, err := normalizeEvent(event)
			if err != nil {
				return err
			}
			reviewID, err := ensureGraphQLReviewID(pending.ReviewID)
			if err != nil {
				return err
			}
			identity, err := shared.identity(args)
			if err != nil {
				return err
			}
			service := reviewsvc.NewService(apiClientFactory(identity.Host))
			status, err := service.Submit(identity, reviewsvc.SubmitInput{
				ReviewID: reviewID,
				Event:    normalizedEvent,
				Body:     pending.Body,
			})
			if err != nil {
				return err
			}
			if status.Success {
				return encodeJSON(cmd, map[string]string{"status": "Review submitted successfully"})
			}
			failure := map[string]interface{}{
				"status": "Review submission failed",
			}
			if len(status.Errors) > 0 {
				failure["errors"] = status.Errors
			}
			if err := encodeJSON(cmd, failure); err != nil {
				return err
			}
			return errors.New("review submission failed")
		},
	}

	pending.bind(cmd)
	cmd.Flags().StringVar(&event, "event", event, "Review submission event (APPROVE, COMMENT, REQUEST_CHANGES)")

	return cmd
}

func normalizeSide(side string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(side))
	switch s {
	case "LEFT", "RIGHT":
		return s, nil
	case "":
		return "", errors.New("side is required")
	default:
		return "", fmt.Errorf("invalid side %q: must be LEFT or RIGHT", side)
	}
}

func normalizeEvent(event string) (string, error) {
	e := strings.ToUpper(strings.TrimSpace(event))
	switch e {
	case "APPROVE", "COMMENT", "REQUEST_CHANGES":
		return e, nil
	default:
		return "", fmt.Errorf("invalid event %q: must be APPROVE, COMMENT, or REQUEST_CHANGES", event)
	}
}

func ensureGraphQLReviewID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return "", errors.New("review id is required")
	}
	if strings.HasPrefix(id, "PRR_") {
		return id, nil
	}
	isNumeric := true
	for _, r := range id {
		if r < '0' || r > '9' {
			isNumeric = false
			break
		}
	}
	if isNumeric {
		return "", fmt.Errorf("--review-id %q is a REST review id; provide the GraphQL review node id (PRR_...)", id)
	}
	return "", fmt.Errorf("--review-id %q is not a GraphQL review node id (expected prefix PRR_)", id)
}

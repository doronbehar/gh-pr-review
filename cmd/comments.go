package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/agynio/gh-pr-review/internal/comments"
	"github.com/agynio/gh-pr-review/internal/resolver"
)

func newCommentsCommand() *cobra.Command {
	opts := &prOptions{}

	cmd := &cobra.Command{
		Use:   "comments",
		Short: "View and reply to pull request review threads",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Help(); err != nil {
				return err
			}
			return errors.New("use 'gh pr-review comments reply' to respond to a review thread; run 'gh pr-review comments view' (alias of 'gh pr-review review view') to locate thread IDs")
		},
	}

	opts.bind(cmd)

	cmd.AddCommand(newCommentsReplyCommand(opts))
	cmd.AddCommand(newCommentsViewCommand(opts))

	return cmd
}

func newCommentsReplyCommand(shared *prOptions) *cobra.Command {
	opts := &commentsReplyOptions{}

	cmd := &cobra.Command{
		Use:   "reply [<number> | <url>]",
		Short: "Reply to a pull request review thread",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identity, err := shared.identity(args)
			if err != nil {
				return err
			}
			return runCommentsReply(cmd, identity, opts)
		},
	}

	cmd.Flags().StringVar(&opts.ThreadID, "thread-id", "", "Review thread identifier to reply to")
	cmd.Flags().StringVar(&opts.ReviewID, "review-id", "", "GraphQL review identifier when replying inside a pending review")
	cmd.Flags().StringVar(&opts.Body, "body", "", "Reply text")
	_ = cmd.MarkFlagRequired("thread-id")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}

type commentsReplyOptions struct {
	ThreadID string
	ReviewID string
	Body     string
}

func runCommentsReply(cmd *cobra.Command, identity resolver.Identity, opts *commentsReplyOptions) error {
	service := comments.NewService(apiClientFactory(identity.Host))

	reply, err := service.Reply(identity, comments.ReplyOptions{
		ThreadID: opts.ThreadID,
		ReviewID: opts.ReviewID,
		Body:     opts.Body,
	})
	if err != nil {
		return err
	}
	if reply.CommentNodeID == "" {
		return errors.New("reply response missing comment node id")
	}
	return encodeJSON(cmd, map[string]string{"comment_node_id": reply.CommentNodeID})
}

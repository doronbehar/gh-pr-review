package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/agynio/gh-pr-review/internal/comments"
	"github.com/agynio/gh-pr-review/internal/resolver"
)

func newReplyCommand(shared *prOptions) *cobra.Command {
	opts := &replyOptions{}

	cmd := &cobra.Command{
		Use:   "reply [<number> | <url>]",
		Short: "Reply to a pull request review thread",
		Long: `Reply to a pull request review thread.

Thread identifiers come from 'gh pr-review view'.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identity, err := shared.identity(args)
			if err != nil {
				return err
			}
			return runReply(cmd, identity, opts)
		},
	}

	cmd.Flags().StringVar(&opts.ThreadID, "thread-id", "", "Review thread identifier to reply to")
	cmd.Flags().StringVar(&opts.ReviewID, "review-id", "", "GraphQL review identifier when replying inside a pending review")
	cmd.Flags().StringVar(&opts.Body, "body", "", "Reply text")
	_ = cmd.MarkFlagRequired("thread-id")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}

type replyOptions struct {
	ThreadID string
	ReviewID string
	Body     string
}

func runReply(cmd *cobra.Command, identity resolver.Identity, opts *replyOptions) error {
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

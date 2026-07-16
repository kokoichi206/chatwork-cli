package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/notation"
	"github.com/kokoichi206/chatwork-cli/internal/output"
)

// postResult は投稿系コマンドの JSON 出力。
// 後続コマンド(reply / edit / delete)へそのまま渡せるよう ID を必ず含める。
type postResult struct {
	RoomID    int    `json:"room_id"`
	MessageID string `json:"message_id"`
}

func (a *app) messagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "messages",
		Aliases: []string{"msg"},
		Short:   "Read and post messages",
	}
	cmd.AddCommand(
		a.messagesReadCmd(),
		a.messagesSendCmd(),
		a.messagesReplyCmd(),
		a.messagesGetCmd(),
		a.messagesEditCmd(),
		a.messagesDeleteCmd(),
		a.messagesMarkReadCmd(),
	)
	return cmd
}

func (a *app) messagesReadCmd() *cobra.Command {
	var (
		limit int
		force bool
	)
	cmd := &cobra.Command{
		Use:   "read <room_id>",
		Short: "Fetch messages (up to 100, oldest first)",
		Long:  "Fetch messages from a room. With --force=false, only messages not yet fetched by this token are returned (empty if none).",
		Args:  exactArgs(1, "cw messages read <room_id> [--limit N] [--force=false]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			msgs, err := client.Messages(cmd.Context(), roomID, force)
			if err != nil {
				return err
			}
			if limit > 0 && len(msgs) > limit {
				msgs = msgs[len(msgs)-limit:]
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			switch f {
			case output.FormatJSON:
				return output.WriteJSON(a.deps.Stdout, msgs)
			case output.FormatText:
				for _, m := range msgs {
					fmt.Fprintf(a.deps.Stdout, "--- %s | %s (account_id=%d) | message_id=%s\n%s\n",
						formatTime(m.SendTime), output.StripControl(m.Account.Name), m.Account.AccountID, m.MessageID, output.StripControl(m.Body))
				}
				return nil
			default:
				rows := make([][]string, 0, len(msgs))
				for _, m := range msgs {
					rows = append(rows, []string{m.MessageID, formatTime(m.SendTime), m.Account.Name, output.Truncate(m.Body, 60)})
				}
				return output.WriteTable(a.deps.Stdout, []string{"MESSAGE_ID", "TIME", "FROM", "BODY"}, rows)
			}
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "show only the newest N messages")
	cmd.Flags().BoolVar(&force, "force", true, "fetch the latest 100 messages regardless of the fetch cursor")
	return cmd
}

func (a *app) messagesSendCmd() *cobra.Command {
	var (
		file       string
		selfUnread bool
	)
	cmd := &cobra.Command{
		Use:   "send <room_id> [body]",
		Short: "Post a message (body as argument, `-` for stdin, or --file)",
		Args:  rangeArgs(1, 2, "cw messages send <room_id> [body|-] [--file <path>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			body, err := a.readBody(args, 1, file)
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			messageID, err := client.PostMessage(cmd.Context(), roomID, body, selfUnread)
			if err != nil {
				return err
			}
			return a.printPostResult(postResult{RoomID: roomID, MessageID: messageID}, "sent")
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read the message body from a file")
	cmd.Flags().BoolVar(&selfUnread, "self-unread", false, "keep the posted message unread for yourself")
	return cmd
}

func (a *app) messagesReplyCmd() *cobra.Command {
	var (
		file      string
		noMention bool
	)
	cmd := &cobra.Command{
		Use:   "reply <room_id> <message_id> [body]",
		Short: "Reply to a message with [rp] notation (mentions the author by default)",
		Long:  "Reply to a message. The [rp] tag alone does not notify the author, so [To:] is added by default; disable with --no-mention.",
		Args:  rangeArgs(2, 3, "cw messages reply <room_id> <message_id> [body|-] [--file <path>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			replyToID := args[1]
			if err := validateMessageID(replyToID); err != nil {
				return err
			}
			body, err := a.readBody(args, 2, file)
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			original, err := client.Message(cmd.Context(), roomID, replyToID)
			if err != nil {
				return fmt.Errorf("fetch reply target: %w", err)
			}
			full := notation.ReplyBody(original.Account.AccountID, roomID, replyToID, body, !noMention)
			messageID, err := client.PostMessage(cmd.Context(), roomID, full, false)
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, struct {
					postResult
					ReplyTo string `json:"reply_to"`
				}{postResult{RoomID: roomID, MessageID: messageID}, replyToID})
			}
			fmt.Fprintf(a.deps.Stdout, "replied: room_id=%d message_id=%s reply_to=%s\n", roomID, messageID, replyToID)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read the message body from a file")
	cmd.Flags().BoolVar(&noMention, "no-mention", false, "omit the [To:] tag (the author will not be notified)")
	return cmd
}

func (a *app) messagesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <room_id> <message_id>",
		Short: "Show a single message",
		Args:  exactArgs(2, "cw messages get <room_id> <message_id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			if err := validateMessageID(args[1]); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			msg, err := client.Message(cmd.Context(), roomID, args[1])
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, msg)
			}
			fmt.Fprintf(a.deps.Stdout, "--- %s | %s (account_id=%d) | message_id=%s\n%s\n",
				formatTime(msg.SendTime), output.StripControl(msg.Account.Name), msg.Account.AccountID, msg.MessageID, output.StripControl(msg.Body))
			return nil
		},
	}
}

func (a *app) messagesEditCmd() *cobra.Command {
	var (
		file string
		yes  bool
	)
	cmd := &cobra.Command{
		Use:   "edit <room_id> <message_id> [body]",
		Short: "Overwrite a message body (own messages only)",
		Args:  rangeArgs(2, 3, "cw messages edit <room_id> <message_id> [body|-] [--file <path>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			messageID := args[1]
			if err := validateMessageID(messageID); err != nil {
				return err
			}
			body, err := a.readBody(args, 2, file)
			if err != nil {
				return err
			}
			if err := a.confirm(fmt.Sprintf("overwrite message %s in room %d", messageID, roomID), yes); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.EditMessage(cmd.Context(), roomID, messageID, body); err != nil {
				return err
			}
			return a.printPostResult(postResult{RoomID: roomID, MessageID: messageID}, "edited")
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read the message body from a file")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}

func (a *app) messagesDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <room_id> <message_id>",
		Short: "Delete a message (own messages only)",
		Args:  exactArgs(2, "cw messages delete <room_id> <message_id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			messageID := args[1]
			if err := validateMessageID(messageID); err != nil {
				return err
			}
			if err := a.confirm(fmt.Sprintf("delete message %s in room %d", messageID, roomID), yes); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.DeleteMessage(cmd.Context(), roomID, messageID); err != nil {
				return err
			}
			return a.printPostResult(postResult{RoomID: roomID, MessageID: messageID}, "deleted")
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}

func (a *app) messagesMarkReadCmd() *cobra.Command {
	var messageID string
	cmd := &cobra.Command{
		Use:   "mark-read <room_id>",
		Short: "Mark messages as read (all, or up to --message)",
		Args:  exactArgs(1, "cw messages mark-read <room_id> [--message <message_id>]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			if messageID != "" {
				if err := validateMessageID(messageID); err != nil {
					return err
				}
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			st, err := client.MarkRead(cmd.Context(), roomID, messageID)
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, st)
			}
			fmt.Fprintf(a.deps.Stdout, "marked read: room_id=%d unread=%d mention=%d\n", roomID, st.UnreadNum, st.MentionNum)
			return nil
		},
	}
	cmd.Flags().StringVar(&messageID, "message", "", "mark as read up to this message_id")
	return cmd
}

func (a *app) printPostResult(res postResult, verb string) error {
	f, err := a.format()
	if err != nil {
		return err
	}
	if f == output.FormatJSON {
		return output.WriteJSON(a.deps.Stdout, res)
	}
	fmt.Fprintf(a.deps.Stdout, "%s: room_id=%d message_id=%s\n", verb, res.RoomID, res.MessageID)
	return nil
}

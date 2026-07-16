package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/output"
)

func (a *app) meCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show your own account information",
		Args:  exactArgs(0, "cw me"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			me, err := client.Me(cmd.Context())
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, me)
			}
			rows := [][]string{
				{"account_id", fmt.Sprint(me.AccountID)},
				{"name", me.Name},
				{"chatwork_id", me.ChatworkID},
				{"organization", me.OrganizationName},
				{"department", me.Department},
				{"my_chat_room_id", fmt.Sprint(me.RoomID)},
			}
			return output.WriteTable(a.deps.Stdout, []string{"FIELD", "VALUE"}, rows)
		},
	}
}

func (a *app) myCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "my",
		Short: "Show your unread and task counts",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show unread / mention / task counts",
		Args:  exactArgs(0, "cw my status"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			st, err := client.MyStatus(cmd.Context())
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
			rows := [][]string{
				{"unread_rooms", fmt.Sprint(st.UnreadRoomNum)},
				{"unread_messages", fmt.Sprint(st.UnreadNum)},
				{"mention_rooms", fmt.Sprint(st.MentionRoomNum)},
				{"mentions", fmt.Sprint(st.MentionNum)},
				{"task_rooms", fmt.Sprint(st.MytaskRoomNum)},
				{"tasks", fmt.Sprint(st.MytaskNum)},
			}
			return output.WriteTable(a.deps.Stdout, []string{"FIELD", "VALUE"}, rows)
		},
	})
	return cmd
}

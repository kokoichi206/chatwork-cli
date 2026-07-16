package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/config"
	"github.com/kokoichi206/chatwork-cli/internal/output"
)

func (a *app) roomsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rooms",
		Short: "List and inspect rooms (group chats / DMs / My Chat)",
	}
	cmd.AddCommand(a.roomsListCmd(), a.roomsGetCmd(), a.roomsMembersCmd(), a.roomsAliasesCmd())
	return cmd
}

// roomsAliasesCmd はプロジェクト設定(.config/chatwork-cli.json)の rooms を表示する。
// agent がリポジトリに紐づく部屋を発見するための入口。
func (a *app) roomsAliasesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "aliases",
		Short: "Show room aliases defined in .config/chatwork-cli.json",
		Args:  exactArgs(0, "cw rooms aliases"),
		RunE: func(_ *cobra.Command, _ []string) error {
			proj, err := a.loadProject()
			if err != nil {
				return err
			}
			if proj == nil {
				return fmt.Errorf("no .config/%s found in this directory or its parents", config.ProjectConfigName)
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, struct {
					Dir   string         `json:"dir"`
					Rooms map[string]int `json:"rooms"`
				}{proj.Dir, proj.Rooms})
			}
			rows := make([][]string, 0, len(proj.Rooms))
			for _, alias := range proj.RoomAliases() {
				rows = append(rows, []string{alias, fmt.Sprint(proj.Rooms[alias])})
			}
			return output.WriteTable(a.deps.Stdout, []string{"ALIAS", "ROOM_ID"}, rows)
		},
	}
}

func (a *app) roomsListCmd() *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all rooms you belong to",
		Args:  exactArgs(0, "cw rooms list [--filter <substr>]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			rooms, err := client.Rooms(cmd.Context())
			if err != nil {
				return err
			}
			if filter != "" {
				filtered := rooms[:0]
				for _, r := range rooms {
					if strings.Contains(strings.ToLower(r.Name), strings.ToLower(filter)) {
						filtered = append(filtered, r)
					}
				}
				rooms = filtered
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, rooms)
			}
			rows := make([][]string, 0, len(rooms))
			for _, r := range rooms {
				rows = append(rows, []string{
					fmt.Sprint(r.RoomID), output.Truncate(r.Name, 40), r.Type, r.Role,
					fmt.Sprint(r.UnreadNum), fmt.Sprint(r.MentionNum), formatTime(r.LastUpdateTime),
				})
			}
			return output.WriteTable(a.deps.Stdout, []string{"ROOM_ID", "NAME", "TYPE", "ROLE", "UNREAD", "MENTION", "LAST_UPDATE"}, rows)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "filter rooms by name substring (case-insensitive)")
	return cmd
}

func (a *app) roomsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <room_id>",
		Short: "Show a room's details",
		Args:  exactArgs(1, "cw rooms get <room_id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			room, err := client.Room(cmd.Context(), roomID)
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, room)
			}
			rows := [][]string{
				{"room_id", fmt.Sprint(room.RoomID)},
				{"name", room.Name},
				{"type", room.Type},
				{"role", room.Role},
				{"unread", fmt.Sprint(room.UnreadNum)},
				{"mention", fmt.Sprint(room.MentionNum)},
				{"messages", fmt.Sprint(room.MessageNum)},
				{"tasks", fmt.Sprint(room.TaskNum)},
				{"files", fmt.Sprint(room.FileNum)},
				{"last_update", formatTime(room.LastUpdateTime)},
				{"description", room.Description},
			}
			return output.WriteTable(a.deps.Stdout, []string{"FIELD", "VALUE"}, rows)
		},
	}
}

func (a *app) roomsMembersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "members <room_id>",
		Short: "List room members (use account_id for mentions and task assignment)",
		Args:  exactArgs(1, "cw rooms members <room_id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			members, err := client.Members(cmd.Context(), roomID)
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, members)
			}
			rows := make([][]string, 0, len(members))
			for _, m := range members {
				rows = append(rows, []string{fmt.Sprint(m.AccountID), m.Name, m.Role, m.ChatworkID})
			}
			return output.WriteTable(a.deps.Stdout, []string{"ACCOUNT_ID", "NAME", "ROLE", "CHATWORK_ID"}, rows)
		},
	}
}

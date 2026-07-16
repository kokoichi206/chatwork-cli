package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/output"
)

func (a *app) tasksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "List and manage tasks",
	}
	cmd.AddCommand(a.tasksListCmd(), a.tasksCreateCmd(), a.tasksDoneCmd())
	return cmd
}

func (a *app) tasksListCmd() *cobra.Command {
	var (
		room   string
		status string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your tasks, or a room's tasks with --room",
		Args:  exactArgs(0, "cw tasks list [--room <room_id>] [--status open|done]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if status != "" && status != "open" && status != "done" {
				return usagef("invalid --status %q (open|done)", status)
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}

			if room != "" {
				roomID, err := a.resolveRoom(room)
				if err != nil {
					return err
				}
				tasks, err := client.RoomTasks(cmd.Context(), roomID, status)
				if err != nil {
					return err
				}
				if f == output.FormatJSON {
					return output.WriteJSON(a.deps.Stdout, tasks)
				}
				rows := make([][]string, 0, len(tasks))
				for _, t := range tasks {
					rows = append(rows, []string{
						fmt.Sprint(t.TaskID), t.Status, t.Account.Name, formatTime(t.LimitTime), output.Truncate(t.Body, 50),
					})
				}
				return output.WriteTable(a.deps.Stdout, []string{"TASK_ID", "STATUS", "ASSIGNEE", "DUE", "BODY"}, rows)
			}

			tasks, err := client.MyTasks(cmd.Context(), status)
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, tasks)
			}
			rows := make([][]string, 0, len(tasks))
			for _, t := range tasks {
				rows = append(rows, []string{
					fmt.Sprint(t.TaskID), fmt.Sprint(t.Room.RoomID), output.Truncate(t.Room.Name, 25),
					t.Status, formatTime(t.LimitTime), output.Truncate(t.Body, 40),
				})
			}
			return output.WriteTable(a.deps.Stdout, []string{"TASK_ID", "ROOM_ID", "ROOM", "STATUS", "DUE", "BODY"}, rows)
		},
	}
	cmd.Flags().StringVar(&room, "room", "", "list tasks of this room (room_id or alias) instead of your own tasks")
	cmd.Flags().StringVar(&status, "status", "open", "filter by status: open|done (empty for both)")
	return cmd
}

func (a *app) tasksCreateCmd() *cobra.Command {
	var (
		to        []int
		body      string
		due       string
		limitType string
	)
	cmd := &cobra.Command{
		Use:   "create <room_id>",
		Short: "Create a task assigned to room members",
		Args:  exactArgs(1, "cw tasks create <room_id> --to <account_id> --body <text> [--due YYYY-MM-DD]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			if len(to) == 0 {
				return usagef("--to <account_id> is required (find IDs with `cw rooms members %d`)", roomID)
			}
			for _, id := range to {
				if id <= 0 {
					return usagef("invalid --to %d: account_id must be a positive integer", id)
				}
			}
			if body == "" {
				return usagef("--body is required")
			}
			switch limitType {
			case "", "none", "date", "time":
			default:
				return usagef("invalid --limit-type %q (none|date|time)", limitType)
			}
			limit, err := parseDue(due)
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			taskIDs, err := client.CreateTask(cmd.Context(), roomID, body, to, limit, limitType)
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, struct {
					RoomID  int   `json:"room_id"`
					TaskIDs []int `json:"task_ids"`
				}{roomID, taskIDs})
			}
			fmt.Fprintf(a.deps.Stdout, "created: room_id=%d task_ids=%v\n", roomID, taskIDs)
			return nil
		},
	}
	cmd.Flags().IntSliceVar(&to, "to", nil, "assignee account_id (repeatable / comma-separated)")
	cmd.Flags().StringVar(&body, "body", "", "task body")
	cmd.Flags().StringVar(&due, "due", "", "due date: YYYY-MM-DD or unix seconds")
	cmd.Flags().StringVar(&limitType, "limit-type", "", "due precision: none|date|time (API default: time)")
	return cmd
}

// parseDue は YYYY-MM-DD(ローカルタイムの 0 時) または unix 秒を受け付ける。空なら期限なし(0)。
// 0 以下の unix 秒は「期限なし」に化けて意図と異なるタスクができるため拒否する。
func parseDue(due string) (int64, error) {
	if due == "" {
		return 0, nil
	}
	if unix, err := strconv.ParseInt(due, 10, 64); err == nil {
		if unix <= 0 {
			return 0, usagef("invalid --due %q: unix seconds must be positive", due)
		}
		return unix, nil
	}
	t, err := time.ParseInLocation("2006-01-02", due, time.Local)
	if err != nil {
		return 0, usagef("invalid --due %q: use YYYY-MM-DD or unix seconds", due)
	}
	return t.Unix(), nil
}

func (a *app) tasksDoneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "done <room_id> <task_id>",
		Short: "Mark a task as done",
		Args:  exactArgs(2, "cw tasks done <room_id> <task_id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			taskID, err := parseID("task_id", args[1])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			if err := client.UpdateTaskStatus(cmd.Context(), roomID, taskID, "done"); err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, struct {
					RoomID int    `json:"room_id"`
					TaskID int    `json:"task_id"`
					Status string `json:"status"`
				}{roomID, taskID, "done"})
			}
			fmt.Fprintf(a.deps.Stdout, "done: room_id=%d task_id=%d\n", roomID, taskID)
			return nil
		},
	}
}

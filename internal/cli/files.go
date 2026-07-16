package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/output"
)

func (a *app) filesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "List files and get download URLs",
	}
	cmd.AddCommand(a.filesListCmd(), a.filesGetCmd())
	return cmd
}

func (a *app) filesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <room_id>",
		Short: "List files uploaded to a room",
		Args:  exactArgs(1, "cw files list <room_id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			files, err := client.Files(cmd.Context(), roomID)
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, files)
			}
			rows := make([][]string, 0, len(files))
			for _, file := range files {
				rows = append(rows, []string{
					fmt.Sprint(file.FileID), output.Truncate(file.Filename, 40),
					fmt.Sprint(file.Filesize), file.Account.Name, formatTime(file.UploadTime),
				})
			}
			return output.WriteTable(a.deps.Stdout, []string{"FILE_ID", "FILENAME", "SIZE", "UPLOADER", "UPLOADED"}, rows)
		},
	}
}

func (a *app) filesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <room_id> <file_id>",
		Short: "Show file info with a download URL (URL expires in 30 seconds)",
		Args:  exactArgs(2, "cw files get <room_id> <file_id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomID, err := a.resolveRoom(args[0])
			if err != nil {
				return err
			}
			fileID, err := parseID("file_id", args[1])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			file, err := client.File(cmd.Context(), roomID, fileID, true)
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, file)
			}
			fmt.Fprintf(a.deps.Stdout, "file_id=%d filename=%s size=%d\ndownload_url=%s\n",
				file.FileID, output.StripControl(file.Filename), file.Filesize, file.DownloadURL)
			return nil
		},
	}
}

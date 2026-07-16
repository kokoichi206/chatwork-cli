package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/output"
	"github.com/kokoichi206/chatwork-cli/internal/update"
)

func (a *app) updateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update cw to the latest GitHub release",
		Args:  exactArgs(0, "cw update"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := update.Run(cmd.Context(), nil, update.Options{
				CurrentVersion: a.deps.Version,
				DryRun:         dryRun,
			})
			if err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, result)
			}
			switch {
			case result.Updated:
				fmt.Fprintf(a.deps.Stdout, "updated: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
			case result.UpdateAvailable:
				fmt.Fprintf(a.deps.Stdout, "update available: %s -> %s (%s)\n",
					result.CurrentVersion, result.LatestVersion, result.AssetName)
			case result.CurrentVersion == result.LatestVersion:
				fmt.Fprintf(a.deps.Stdout, "already up to date (%s)\n", result.CurrentVersion)
			default:
				fmt.Fprintf(a.deps.Stdout, "current version %s is newer than the latest release %s\n",
					result.CurrentVersion, result.LatestVersion)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "check the latest release without installing")
	return cmd
}

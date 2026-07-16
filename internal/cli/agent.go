package cli

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/output"
)

// skillMD が正本。リポジトリ直下の .claude/skills/chatwork-cli/SKILL.md はこの複製で、
// 一致することをテストで担保している(TestRepoSkillMatchesEmbedded)。
//
//go:embed SKILL.md
var skillMD string

const agentsMDSnippet = `## Chatwork (cw)

Use the ` + "`cw`" + ` CLI to read and post Chatwork messages as the authenticated user.

- Run ` + "`cw docs`" + ` for the full machine-oriented reference (workflows, notation, cautions).
- Always pass ` + "`--output json`" + `. Exit codes: 0 success, 1 API error, 2 usage error.
- Destructive commands (` + "`messages edit|delete`" + `, ` + "`auth remove`" + `) need ` + "`--yes`" + ` in non-interactive mode.
- No account configured? Ask the user to run ` + "`cw auth guide`" + `.
`

func (a *app) agentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Set up AI-agent integration for cw",
	}
	cmd.AddCommand(a.agentInitCmd())
	return cmd
}

type agentInitResult struct {
	Scope  string `json:"scope"`
	Path   string `json:"path"`
	Status string `json:"status"` // installed | updated | up-to-date
}

func (a *app) agentInitCmd() *cobra.Command {
	var (
		scope    string
		yes      bool
		agentsMD bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install the Claude Code skill for cw",
		Long: "Install the bundled Claude Code skill so agents discover how to use cw.\n" +
			"--scope repo writes ./.claude/skills/chatwork-cli/SKILL.md (commit it to share with your team).\n" +
			"--scope user writes ~/.claude/skills/chatwork-cli/SKILL.md (available in all your projects).\n" +
			"For agents that read AGENTS.md instead, print a snippet with --agents-md and paste it yourself.",
		Args: exactArgs(0, "cw agent init [--scope repo|user] [--yes] [--agents-md]"),
		RunE: func(_ *cobra.Command, _ []string) error {
			if agentsMD {
				fmt.Fprint(a.deps.Stdout, agentsMDSnippet)
				return nil
			}
			path, err := a.skillPath(scope)
			if err != nil {
				return err
			}

			status := "installed"
			existing, err := os.ReadFile(path)
			switch {
			case err == nil && bytes.Equal(existing, []byte(skillMD)):
				status = "up-to-date"
			case err == nil:
				if err := a.confirm(fmt.Sprintf("overwrite existing %s", path), yes); err != nil {
					return err
				}
				status = "updated"
			case !errors.Is(err, fs.ErrNotExist):
				return err
			}

			if status != "up-to-date" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(skillMD), 0o644); err != nil {
					return err
				}
			}

			res := agentInitResult{Scope: scope, Path: path, Status: status}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, res)
			}
			fmt.Fprintf(a.deps.Stdout, "%s: %s\n", res.Status, res.Path)
			if scope == "repo" && status != "up-to-date" {
				fmt.Fprintln(a.deps.Stdout, "commit .claude/skills/chatwork-cli/ to share it with your team")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "repo", "where to install the skill: repo (./.claude/skills) or user (~/.claude/skills)")
	cmd.Flags().BoolVar(&yes, "yes", false, "overwrite an existing skill file without confirmation")
	cmd.Flags().BoolVar(&agentsMD, "agents-md", false, "print an AGENTS.md snippet to stdout instead of installing the skill")
	return cmd
}

func (a *app) skillPath(scope string) (string, error) {
	var base string
	switch scope {
	case "repo":
		base = a.deps.WorkDir
		if base == "" {
			base = "."
		}
	case "user":
		base = a.deps.Home
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve home dir: %w", err)
			}
			base = home
		}
	default:
		return "", usagef("invalid --scope %q (repo|user)", scope)
	}
	return filepath.Join(base, ".claude", "skills", "chatwork-cli", "SKILL.md"), nil
}

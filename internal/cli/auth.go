package cli

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/config"
	"github.com/kokoichi206/chatwork-cli/internal/output"
)

// authAccountView は auth 系コマンドの JSON 出力。トークンは決して含めない。
type authAccountView struct {
	Alias     string `json:"alias"`
	AccountID int    `json:"account_id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

func (a *app) authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Chatwork accounts (personal API tokens)",
	}
	cmd.AddCommand(a.authLoginCmd(), a.authListCmd(), a.authSetDefaultCmd(), a.authRemoveCmd(), a.authStatusCmd(), a.authGuideCmd())
	return cmd
}

func (a *app) authLoginCmd() *cobra.Command {
	var (
		name       string
		tokenStdin bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Register an API token (validated via GET /me)",
		Long:  "Register a Chatwork personal API token. Get one at: Chatwork > 動作設定 > API Token.\nThe token acts as you; messages posted with it appear as your own.",
		Args:  exactArgs(0, "cw auth login [--name <alias>] [--token-stdin]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			token, err := a.readToken(tokenStdin)
			if err != nil {
				return err
			}
			me, err := a.newClient(token).Me(cmd.Context())
			if err != nil {
				return fmt.Errorf("token validation failed: %w", err)
			}

			cfg, path, err := a.loadConfig()
			if err != nil {
				return err
			}
			cfg.Accounts[name] = config.Account{Token: token, AccountID: me.AccountID, Name: me.Name}
			if cfg.Default == "" {
				cfg.Default = name
			}
			if err := cfg.Save(path); err != nil {
				return err
			}

			view := authAccountView{Alias: name, AccountID: me.AccountID, Name: me.Name, IsDefault: cfg.Default == name}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, view)
			}
			fmt.Fprintf(a.deps.Stdout, "Logged in as %s (account_id=%d, alias=%s)\n", me.Name, me.AccountID, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "default", "alias for this account")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read the API token from stdin (required in non-interactive mode)")
	return cmd
}

func (a *app) readToken(tokenStdin bool) (string, error) {
	if tokenStdin {
		data, err := io.ReadAll(a.deps.Stdin)
		if err != nil {
			return "", err
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", usagef("empty token on stdin")
		}
		return token, nil
	}
	if !a.deps.StdinIsTTY {
		return "", usagef("no TTY detected; pipe the token with --token-stdin (see `cw auth guide` for how to get one)")
	}
	fmt.Fprint(a.deps.Stderr, tokenGuideShort)
	fmt.Fprint(a.deps.Stderr, "Chatwork API token (入力は表示されません): ")

	var token string
	if a.deps.ReadPassword != nil {
		data, err := a.deps.ReadPassword()
		fmt.Fprintln(a.deps.Stderr) // ReadPassword は改行をエコーしないため補う
		if err != nil {
			return "", err
		}
		token = strings.TrimSpace(string(data))
	} else {
		line, err := bufio.NewReader(a.deps.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		token = strings.TrimSpace(line)
	}
	if token == "" {
		return "", usagef("empty token")
	}
	return token, nil
}

func (a *app) authListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered accounts",
		Args:  exactArgs(0, "cw auth list"),
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _, err := a.loadConfig()
			if err != nil {
				return err
			}
			aliases := make([]string, 0, len(cfg.Accounts))
			for alias := range cfg.Accounts {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			views := make([]authAccountView, 0, len(aliases))
			for _, alias := range aliases {
				acct := cfg.Accounts[alias]
				views = append(views, authAccountView{
					Alias:     alias,
					AccountID: acct.AccountID,
					Name:      acct.Name,
					IsDefault: alias == cfg.Default,
				})
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, views)
			}
			rows := make([][]string, 0, len(views))
			for _, v := range views {
				def := ""
				if v.IsDefault {
					def = "*"
				}
				rows = append(rows, []string{v.Alias, fmt.Sprint(v.AccountID), v.Name, def})
			}
			return output.WriteTable(a.deps.Stdout, []string{"ALIAS", "ACCOUNT_ID", "NAME", "DEFAULT"}, rows)
		},
	}
}

func (a *app) authSetDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-default <alias>",
		Short: "Set the default account",
		Args:  exactArgs(1, "cw auth set-default <alias>"),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := a.loadConfig()
			if err != nil {
				return err
			}
			alias := args[0]
			if _, ok := cfg.Accounts[alias]; !ok {
				return usagef("account %q not found; run `cw auth list`", alias)
			}
			cfg.Default = alias
			if err := cfg.Save(path); err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, struct {
					Default string `json:"default"`
				}{alias})
			}
			fmt.Fprintf(a.deps.Stdout, "default account: %s\n", alias)
			return nil
		},
	}
}

func (a *app) authRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <alias>",
		Short: "Remove an account from local config",
		Args:  exactArgs(1, "cw auth remove <alias>"),
		RunE: func(_ *cobra.Command, args []string) error {
			alias := args[0]
			cfg, path, err := a.loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Accounts[alias]; !ok {
				return usagef("account %q not found; run `cw auth list`", alias)
			}
			if err := a.confirm(fmt.Sprintf("remove account %q", alias), yes); err != nil {
				return err
			}
			delete(cfg.Accounts, alias)
			if cfg.Default == alias {
				cfg.Default = ""
			}
			if err := cfg.Save(path); err != nil {
				return err
			}
			f, err := a.format()
			if err != nil {
				return err
			}
			if f == output.FormatJSON {
				return output.WriteJSON(a.deps.Stdout, struct {
					Removed string `json:"removed"`
					Default string `json:"default,omitempty"`
				}{alias, cfg.Default})
			}
			fmt.Fprintf(a.deps.Stdout, "removed account: %s\n", alias)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}

func (a *app) authStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Verify the current token via GET /me",
		Args:  exactArgs(0, "cw auth status"),
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
			fmt.Fprintf(a.deps.Stdout, "authenticated as %s (account_id=%d, chatwork_id=%s)\n", me.Name, me.AccountID, me.ChatworkID)
			return nil
		},
	}
}

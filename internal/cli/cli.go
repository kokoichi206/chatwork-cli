// Package cli は cobra コマンド定義の薄い層。
// 入出力の整形とバリデーションのみを担い、API 呼び出しは internal/chatwork に委譲する。
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/chatwork"
	"github.com/kokoichi206/chatwork-cli/internal/config"
	"github.com/kokoichi206/chatwork-cli/internal/output"
)

// EnvToken が設定されている場合、設定ファイルよりも優先して使う。
// CI や agent のワンショット実行で accounts.json を作らずに済ませるための経路。
const EnvToken = "CHATWORK_API_TOKEN"

type Deps struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
	ConfigPath string // 空なら config.Path()
	BaseURL    string // 空なら chatwork.DefaultBaseURL
	// TTY 判定は用途ごとに分ける: 出力形式の既定は stdout、対話(確認・トークン入力)は stdin で決める。
	// 片方だけパイプされた実行(例: `cw ... > out.json` や `echo tok | cw auth login`)で混線しないため。
	StdoutIsTTY bool
	StdinIsTTY  bool
	Getenv      func(string) string
	Version     string
	Home        string // 空なら os.UserHomeDir()。agent init --scope user の書き込み先
	WorkDir     string // 空ならカレントディレクトリ。agent init --scope repo の書き込み先
	DataDir     string // 空なら XDG_DATA_HOME(既定 ~/.local/share)/chatwork-cli。cw sync のローカル履歴保存先

	// ReadPassword は TTY でのトークン入力に使う(エコーなし)。
	// nil の場合は Stdin からの行読みにフォールバックする(テスト用)。
	ReadPassword func() ([]byte, error)
}

// UsageError は使い方の誤り(終了コード 2)を API エラー(終了コード 1)と区別する。
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usagef(format string, a ...any) error {
	return &UsageError{Err: fmt.Errorf(format, a...)}
}

// ExitCode はエラーを終了コードに写像する: 1=API/実行時エラー, 2=使い方エラー。
// cobra は未知のサブコマンドを素のエラーで返すため、文字列前置きでも判定する。
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var usageErr *UsageError
	if errors.As(err, &usageErr) || strings.HasPrefix(err.Error(), "unknown command") {
		return 2
	}
	return 1
}

type app struct {
	deps Deps

	accountFlag string
	outputFlag  string

	project       *config.Project
	projectLoaded bool
}

func New(deps Deps) *cobra.Command {
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	a := &app{deps: deps}

	root := &cobra.Command{
		Use:           "cw",
		Short:         "Chatwork CLI for humans and AI agents",
		Long:          "cw is a Chatwork CLI. It posts as the authenticated user (personal API token).\nRun `cw docs` for an AI-agent oriented full reference.",
		Version:       deps.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.SetIn(nopReadCloser{deps.Stdin})
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Err: err}
	})

	pf := root.PersistentFlags()
	pf.StringVar(&a.accountFlag, "account", "", "account alias to use (default: the one set by 'cw auth set-default', or the only account)")
	pf.StringVarP(&a.outputFlag, "output", "o", "", "output format: json|table|text (default: table on TTY, json otherwise)")

	root.AddCommand(
		a.authCmd(),
		a.roomsCmd(),
		a.messagesCmd(),
		a.syncCmd(),
		a.tasksCmd(),
		a.filesCmd(),
		a.meCmd(),
		a.myCmd(),
		a.docsCmd(),
		a.agentCmd(),
		a.updateCmd(),
	)
	return root
}

type nopReadCloser struct{ io.Reader }

func (nopReadCloser) Close() error { return nil }

func (a *app) configPath() (string, error) {
	if a.deps.ConfigPath != "" {
		return a.deps.ConfigPath, nil
	}
	return config.Path()
}

func (a *app) loadConfig() (*config.Config, string, error) {
	path, err := a.configPath()
	if err != nil {
		return nil, "", err
	}
	if warn := config.PermWarning(path); warn != "" {
		fmt.Fprintln(a.deps.Stderr, warn)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}
	return cfg, path, nil
}

func (a *app) newClient(token string) *chatwork.Client {
	opts := []chatwork.Option{}
	if a.deps.BaseURL != "" {
		opts = append(opts, chatwork.WithBaseURL(a.deps.BaseURL))
	}
	return chatwork.New(token, opts...)
}

// loadProject はリポジトリスコープ設定(.config/chatwork-cli.json)を一度だけ読む。
// 見つからない場合は nil を返し、各機能は通常動作にフォールバックする
// (プロジェクト設定はオプショナルな入力であり、必須データではない)。
func (a *app) loadProject() (*config.Project, error) {
	if a.projectLoaded {
		return a.project, nil
	}
	workDir := a.deps.WorkDir
	if workDir == "" {
		workDir = "."
	}
	home := a.deps.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	proj, err := config.FindProject(workDir, home)
	if err != nil {
		return nil, err
	}
	a.project = proj
	a.projectLoaded = true
	return proj, nil
}

// client はトークンを解決して API クライアントを返す。
// --account 指定がない場合のみ環境変数 CHATWORK_API_TOKEN が設定ファイルより優先される。
func (a *app) client() (*chatwork.Client, error) {
	if a.accountFlag == "" {
		if token := a.deps.Getenv(EnvToken); token != "" {
			return a.newClient(token), nil
		}
	}
	cfg, _, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	_, acct, err := cfg.Resolve(a.accountFlag)
	if err != nil {
		return nil, err
	}
	return a.newClient(acct.Token), nil
}

// resolveRoom は room 引数を解決する。数値なら room_id、それ以外は
// プロジェクト設定 rooms のエイリアスとして引く。
func (a *app) resolveRoom(arg string) (int, error) {
	if id, err := strconv.Atoi(arg); err == nil {
		if id <= 0 {
			return 0, usagef("invalid room_id %q: must be a positive integer", arg)
		}
		return id, nil
	}
	proj, err := a.loadProject()
	if err != nil {
		return 0, err
	}
	if proj == nil {
		return 0, usagef("invalid room %q: not a room_id, and no .config/chatwork-cli.json defines room aliases", arg)
	}
	if id, ok := proj.Rooms[arg]; ok {
		if id <= 0 {
			return 0, usagef("room alias %q maps to invalid room_id %d in %s",
				arg, id, filepath.Join(proj.Dir, ".config", config.ProjectConfigName))
		}
		return id, nil
	}
	return 0, usagef("unknown room alias %q; defined in %s: %s",
		arg, filepath.Join(proj.Dir, ".config", config.ProjectConfigName), strings.Join(proj.RoomAliases(), ", "))
}

func (a *app) format() (output.Format, error) {
	if a.outputFlag == "" {
		return output.Default(a.deps.StdoutIsTTY), nil
	}
	f, err := output.Parse(a.outputFlag)
	if err != nil {
		return "", &UsageError{Err: err}
	}
	return f, nil
}

// confirm は破壊的操作のガード。非 TTY では --yes を必須にして agent の誤爆を防ぐ。
func (a *app) confirm(action string, yes bool) error {
	if yes {
		return nil
	}
	if !a.deps.StdinIsTTY {
		return usagef("refusing to %s without confirmation; pass --yes in non-interactive mode", action)
	}
	fmt.Fprintf(a.deps.Stderr, "%s? [y/N]: ", action)
	line, err := bufio.NewReader(a.deps.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return fmt.Errorf("aborted")
}

// readBody は本文を フラグ --file > 引数 "-"(stdin) > 引数そのまま の順で解決する。
func (a *app) readBody(args []string, argIndex int, file string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	if len(args) <= argIndex {
		return "", usagef("message body is required (as an argument, `-` for stdin, or --file)")
	}
	body := args[argIndex]
	if body == "-" {
		data, err := io.ReadAll(a.deps.Stdin)
		if err != nil {
			return "", err
		}
		body = strings.TrimRight(string(data), "\n")
	}
	if body == "" {
		return "", usagef("message body is empty")
	}
	return body, nil
}

func exactArgs(n int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return usagef("expected %d argument(s): %s", n, usage)
		}
		return nil
	}
}

func rangeArgs(min, max int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < min || len(args) > max {
			return usagef("expected %d-%d argument(s): %s", min, max, usage)
		}
		return nil
	}
}

// validateMessageID は message_id が数値文字列であることを検証する。
// int64 を超えうるため数値へは変換せず、文字列のまま扱う。
func validateMessageID(s string) error {
	if s == "" {
		return usagef("message_id is empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return usagef("invalid message_id %q: must be a numeric string", s)
		}
	}
	return nil
}

func parseID(name, s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, usagef("invalid %s %q: must be a positive integer", name, s)
	}
	return id, nil
}

func formatTime(unix int64) string {
	if unix == 0 {
		return "-"
	}
	return time.Unix(unix, 0).Local().Format("2006-01-02 15:04")
}

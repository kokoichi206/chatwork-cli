package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/cli"
	"github.com/kokoichi206/chatwork-cli/internal/config"
)

type cliResult struct {
	stdout string
	stderr string
	err    error
}

type cliOpts struct {
	cfg     *config.Config
	isTTY   bool
	stdin   string
	env     map[string]string
	dataDir string
}

func runCLI(t *testing.T, handler http.Handler, opts cliOpts, args ...string) cliResult {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	configPath := filepath.Join(t.TempDir(), "accounts.json")
	if opts.cfg != nil {
		if err := opts.cfg.Save(configPath); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	root := cli.New(cli.Deps{
		Stdout:      &stdout,
		Stderr:      &stderr,
		Stdin:       strings.NewReader(opts.stdin),
		ConfigPath:  configPath,
		BaseURL:     srv.URL,
		StdoutIsTTY: opts.isTTY,
		StdinIsTTY:  opts.isTTY,
		Getenv:      func(key string) string { return opts.env[key] },
		Version:     "test",
		DataDir:     opts.dataDir,
	})
	root.SetArgs(args)
	err := root.Execute()
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func singleAccount() *config.Config {
	return &config.Config{
		Default:  "work",
		Accounts: map[string]config.Account{"work": {Token: "tok", AccountID: 1, Name: "田中"}},
	}
}

func TestAuthLoginSavesValidatedToken(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-chatworktoken") != "secret-token" {
			t.Errorf("token = %q", r.Header.Get("x-chatworktoken"))
		}
		fmt.Fprint(w, `{"account_id":100,"name":"田中"}`)
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	configPath := filepath.Join(t.TempDir(), "accounts.json")

	var stdout, stderr bytes.Buffer
	root := cli.New(cli.Deps{
		Stdout: &stdout, Stderr: &stderr,
		Stdin:      strings.NewReader("secret-token\n"),
		ConfigPath: configPath,
		BaseURL:    srv.URL,
		Getenv:     func(string) string { return "" },
	})
	root.SetArgs([]string{"auth", "login", "--name", "work", "--token-stdin", "--output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v (stderr: %s)", err, stderr.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	acct, ok := cfg.Accounts["work"]
	if !ok || acct.Token != "secret-token" || acct.AccountID != 100 {
		t.Errorf("saved account = %+v", cfg.Accounts)
	}
	if cfg.Default != "work" {
		t.Errorf("default = %q, want work (first login becomes default)", cfg.Default)
	}
	if strings.Contains(stdout.String(), "secret-token") {
		t.Error("token must not appear in output")
	}
}

func TestAuthLoginRequiresStdinFlagWhenNotTTY(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{stdin: "tok\n"}, "auth", "login")
	var usageErr *cli.UsageError
	if !errors.As(res.err, &usageErr) {
		t.Fatalf("error = %v, want UsageError", res.err)
	}
}

func TestRoomsListJSONAndFilter(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"room_id":1,"name":"開発チーム","type":"group"},
			{"room_id":2,"name":"営業","type":"group"}
		]`)
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, "rooms", "list", "--filter", "開発", "--output", "json")
	if res.err != nil {
		t.Fatalf("error = %v (stderr: %s)", res.err, res.stderr)
	}
	var rooms []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &rooms); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, res.stdout)
	}
	if len(rooms) != 1 || rooms[0]["room_id"].(float64) != 1 {
		t.Errorf("rooms = %v", rooms)
	}
}

func TestPipedOutputDefaultsToJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"room_id":1,"name":"a","type":"group"}]`)
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount(), isTTY: false}, "rooms", "list")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if !strings.HasPrefix(strings.TrimSpace(res.stdout), "[") {
		t.Errorf("non-TTY default output is not JSON:\n%s", res.stdout)
	}
}

func TestMessagesSendReturnsIDs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rooms/42/messages" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"message_id":"999"}`)
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, "messages", "send", "42", "hello", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	var out struct {
		RoomID    int    `json:"room_id"`
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, res.stdout)
	}
	if out.RoomID != 42 || out.MessageID != "999" {
		t.Errorf("out = %+v", out)
	}
}

func TestMessagesSendBodyFromStdin(t *testing.T) {
	var posted string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		posted = r.PostForm.Get("body")
		fmt.Fprint(w, `{"message_id":"1"}`)
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount(), stdin: "1行目\n2行目\n"}, "messages", "send", "42", "-")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if posted != "1行目\n2行目" {
		t.Errorf("posted body = %q", posted)
	}
}

func TestMessagesReplyBuildsNotation(t *testing.T) {
	var posted string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rooms/42/messages/777":
			fmt.Fprint(w, `{"message_id":"777","account":{"account_id":555,"name":"佐藤"},"body":"元メッセージ","send_time":1700000000}`)
		case r.Method == http.MethodPost && r.URL.Path == "/rooms/42/messages":
			_ = r.ParseForm()
			posted = r.PostForm.Get("body")
			fmt.Fprint(w, `{"message_id":"1000"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, "messages", "reply", "42", "777", "確認しました", "--output", "json")
	if res.err != nil {
		t.Fatalf("error = %v (stderr: %s)", res.err, res.stderr)
	}
	want := "[rp aid=555 to=42-777][To:555]\n確認しました"
	if posted != want {
		t.Errorf("posted body = %q, want %q", posted, want)
	}
	if !strings.Contains(res.stdout, `"reply_to": "777"`) {
		t.Errorf("stdout = %s", res.stdout)
	}
}

func TestMessagesReplyNoMention(t *testing.T) {
	var posted string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"message_id":"777","account":{"account_id":555,"name":"佐藤"}}`)
			return
		}
		_ = r.ParseForm()
		posted = r.PostForm.Get("body")
		fmt.Fprint(w, `{"message_id":"1000"}`)
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, "messages", "reply", "42", "777", "ok", "--no-mention")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if strings.Contains(posted, "[To:") {
		t.Errorf("posted body should not contain [To:]: %q", posted)
	}
}

func TestMessagesDeleteRequiresYesWhenNotTTY(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		fmt.Fprint(w, `{"message_id":"777"}`)
	})

	res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, "messages", "delete", "42", "777")
	var usageErr *cli.UsageError
	if !errors.As(res.err, &usageErr) {
		t.Fatalf("error = %v, want UsageError", res.err)
	}
	if called {
		t.Error("API must not be called without confirmation")
	}

	res = runCLI(t, handler, cliOpts{cfg: singleAccount()}, "messages", "delete", "42", "777", "--yes")
	if res.err != nil {
		t.Fatalf("with --yes: error = %v", res.err)
	}
	if !called {
		t.Error("API should be called with --yes")
	}
}

func TestEnvTokenOverridesConfig(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-chatworktoken"); got != "env-token" {
			t.Errorf("token = %q, want env-token", got)
		}
		fmt.Fprint(w, `{"account_id":1,"name":"n"}`)
	})
	res := runCLI(t, handler, cliOpts{
		cfg: singleAccount(),
		env: map[string]string{cli.EnvToken: "env-token"},
	}, "me")
	if res.err != nil {
		t.Fatal(res.err)
	}
}

func TestUnknownAccountIsUsageError(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, "me", "--account", "nope")
	if res.err == nil || !strings.Contains(res.err.Error(), "not found") {
		t.Fatalf("error = %v", res.err)
	}
}

func TestTasksListMineByDefault(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/my/tasks" {
			t.Errorf("path = %s, want /my/tasks", r.URL.Path)
		}
		if r.URL.Query().Get("status") != "open" {
			t.Errorf("status = %q, want open", r.URL.Query().Get("status"))
		}
		fmt.Fprint(w, `[{"task_id":3,"room":{"room_id":5,"name":"r"},"body":"b","status":"open"}]`)
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, "tasks", "list", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if !strings.Contains(res.stdout, `"task_id": 3`) {
		t.Errorf("stdout = %s", res.stdout)
	}
}

func TestAuthListDoesNotLeakTokens(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, "auth", "list", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if strings.Contains(res.stdout, "tok") {
		t.Errorf("token leaked in output: %s", res.stdout)
	}
	if !strings.Contains(res.stdout, `"alias": "work"`) {
		t.Errorf("stdout = %s", res.stdout)
	}
}

func TestExitCodeClassification(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"missing args", []string{"messages", "send"}, 2},
		{"unknown command", []string{"badcmd"}, 2},
		{"unknown flag", []string{"rooms", "list", "--nope"}, 2},
		{"api error", []string{"me"}, 1},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"errors":["Invalid API Token"]}`)
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, tt.args...)
			if res.err == nil {
				t.Fatal("want error")
			}
			if got := cli.ExitCode(res.err); got != tt.want {
				t.Errorf("ExitCode(%v) = %d, want %d", res.err, got, tt.want)
			}
		})
	}
}

func TestDocsCoversCoreCommands(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{}, "docs")
	if res.err != nil {
		t.Fatal(res.err)
	}
	for _, want := range []string{"cw messages reply", "cw rooms members", "cw sync", "--local", "[To:", "rp aid=", "--output json", "CHATWORK_API_TOKEN"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("docs missing %q", want)
		}
	}
}

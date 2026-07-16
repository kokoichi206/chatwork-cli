package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/cli"
)

func runCLIWithTTY(t *testing.T, handler http.Handler, stdin string, stdoutTTY, stdinTTY bool, args ...string) cliResult {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	configPath := filepath.Join(t.TempDir(), "accounts.json")
	if err := singleAccount().Save(configPath); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	root := cli.New(cli.Deps{
		Stdout:      &stdout,
		Stderr:      &stderr,
		Stdin:       strings.NewReader(stdin),
		ConfigPath:  configPath,
		BaseURL:     srv.URL,
		StdoutIsTTY: stdoutTTY,
		StdinIsTTY:  stdinTTY,
		Getenv:      func(string) string { return "" },
	})
	root.SetArgs(args)
	err := root.Execute()
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func TestMessageIDValidation(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		fmt.Fprint(w, `{}`)
	})
	tests := []struct {
		name string
		args []string
	}{
		{"delete empty id", []string{"messages", "delete", "42", "", "--yes"}},
		{"delete non-numeric id", []string{"messages", "delete", "42", "abc", "--yes"}},
		{"get non-numeric id", []string{"messages", "get", "42", "abc"}},
		{"edit non-numeric id", []string{"messages", "edit", "42", "abc", "body", "--yes"}},
		{"reply non-numeric id", []string{"messages", "reply", "42", "abc", "body"}},
		{"mark-read non-numeric --message", []string{"messages", "mark-read", "42", "--message", "abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, tt.args...)
			var usageErr *cli.UsageError
			if !errors.As(res.err, &usageErr) {
				t.Fatalf("error = %v, want UsageError", res.err)
			}
			if called {
				t.Error("API must not be called with an invalid message_id")
			}
		})
	}
}

// メッセージ 0 件(API は 204)でも JSON 出力は null ではなく [] になること。
func TestMessagesReadEmptyIsJSONArray(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, "messages", "read", "42", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if got := strings.TrimSpace(res.stdout); got != "[]" {
		t.Errorf("stdout = %q, want []", got)
	}
}

func TestAuthSetDefaultAndRemoveSupportJSON(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, "auth", "set-default", "work", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if !strings.Contains(res.stdout, `"default": "work"`) {
		t.Errorf("set-default stdout = %s", res.stdout)
	}

	res = runCLI(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, "auth", "remove", "work", "--yes", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if !strings.Contains(res.stdout, `"removed": "work"`) {
		t.Errorf("remove stdout = %s", res.stdout)
	}
}

func TestTasksCreateInputValidation(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		fmt.Fprint(w, `{"task_ids":[1]}`)
	})
	tests := []struct {
		name string
		args []string
	}{
		{"negative --to", []string{"tasks", "create", "42", "--to", "-1", "--body", "b"}},
		{"zero --to", []string{"tasks", "create", "42", "--to", "0", "--body", "b"}},
		{"negative --due unix", []string{"tasks", "create", "42", "--to", "10", "--body", "b", "--due", "-5"}},
		{"bogus --limit-type", []string{"tasks", "create", "42", "--to", "10", "--body", "b", "--limit-type", "hour"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runCLI(t, handler, cliOpts{cfg: singleAccount()}, tt.args...)
			var usageErr *cli.UsageError
			if !errors.As(res.err, &usageErr) {
				t.Fatalf("error = %v, want UsageError", res.err)
			}
			if called {
				t.Error("API must not be called with invalid task inputs")
			}
		})
	}
}

func TestProjectAliasWithInvalidRoomIDIsUsageError(t *testing.T) {
	workDir := t.TempDir()
	writeProjectConfig(t, workDir, `{"rooms":{"main":0}}`)

	res := runCLIInDir(t, http.NotFoundHandler(), workDir, "messages", "read", "main")
	var usageErr *cli.UsageError
	if !errors.As(res.err, &usageErr) {
		t.Fatalf("error = %v, want UsageError", res.err)
	}
	if !strings.Contains(res.err.Error(), "invalid room_id 0") {
		t.Errorf("error = %v", res.err)
	}
}

// stdout がパイプ(非TTY)でも stdin が TTY なら対話ログイン・確認プロンプトは使えること。
func TestStdinTTYControlsInteractivityIndependently(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"message_id":"777"}`)
	})
	res := runCLIWithTTY(t, handler, "y\n", false, true, "messages", "delete", "42", "777")
	if res.err != nil {
		t.Fatalf("delete with stdin TTY should prompt and accept 'y': %v", res.err)
	}
	if !strings.Contains(res.stderr, "delete message 777") {
		t.Errorf("confirmation prompt missing on stderr: %s", res.stderr)
	}
}

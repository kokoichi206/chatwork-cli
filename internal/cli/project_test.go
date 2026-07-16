package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/cli"
	"github.com/kokoichi206/chatwork-cli/internal/config"
)

func writeProjectConfig(t *testing.T, dir, content string) {
	t.Helper()
	confDir := filepath.Join(dir, ".config")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "chatwork-cli.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCLIInDir(t *testing.T, handler http.Handler, workDir string, args ...string) cliResult {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	configPath := filepath.Join(t.TempDir(), "accounts.json")
	cfg := &config.Config{
		Default:  "work",
		Accounts: map[string]config.Account{"work": {Token: "tok", AccountID: 1, Name: "田中"}},
	}
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	root := cli.New(cli.Deps{
		Stdout:     &stdout,
		Stderr:     &stderr,
		Stdin:      strings.NewReader(""),
		ConfigPath: configPath,
		BaseURL:    srv.URL,
		Getenv:     func(string) string { return "" },
		WorkDir:    workDir,
		Home:       t.TempDir(),
	})
	root.SetArgs(args)
	err := root.Execute()
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func TestRoomAliasResolvesToID(t *testing.T) {
	workDir := t.TempDir()
	writeProjectConfig(t, workDir, `{"rooms":{"main":405354226}}`)

	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"message_id":"1"}`)
	})
	res := runCLIInDir(t, handler, workDir, "messages", "send", "main", "hello")
	if res.err != nil {
		t.Fatalf("error = %v (stderr: %s)", res.err, res.stderr)
	}
	if gotPath != "/rooms/405354226/messages" {
		t.Errorf("path = %q, want /rooms/405354226/messages", gotPath)
	}
}

func TestNumericRoomBypassesAliases(t *testing.T) {
	workDir := t.TempDir()
	writeProjectConfig(t, workDir, `{"rooms":{"main":111}}`)

	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `[]`)
	})
	res := runCLIInDir(t, handler, workDir, "messages", "read", "222")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if gotPath != "/rooms/222/messages" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestUnknownAliasListsDefined(t *testing.T) {
	workDir := t.TempDir()
	writeProjectConfig(t, workDir, `{"rooms":{"alerts":2,"main":1}}`)

	res := runCLIInDir(t, http.NotFoundHandler(), workDir, "messages", "read", "nope")
	var usageErr *cli.UsageError
	if !errors.As(res.err, &usageErr) {
		t.Fatalf("error = %v, want UsageError", res.err)
	}
	if !strings.Contains(res.err.Error(), "alerts, main") {
		t.Errorf("error should list defined aliases: %v", res.err)
	}
}

func TestAliasWithoutProjectConfigIsUsageError(t *testing.T) {
	res := runCLIInDir(t, http.NotFoundHandler(), t.TempDir(), "messages", "read", "main")
	var usageErr *cli.UsageError
	if !errors.As(res.err, &usageErr) {
		t.Fatalf("error = %v, want UsageError", res.err)
	}
	if !strings.Contains(res.err.Error(), "chatwork-cli.json") {
		t.Errorf("error should mention the project config file: %v", res.err)
	}
}

func TestRoomsAliasesShowsProjectRooms(t *testing.T) {
	workDir := t.TempDir()
	writeProjectConfig(t, workDir, `{"rooms":{"main":405354226,"alerts":123}}`)

	res := runCLIInDir(t, http.NotFoundHandler(), workDir, "rooms", "aliases", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	for _, want := range []string{`"main": 405354226`, `"alerts": 123`, `"dir"`} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestRoomsAliasesWithoutProjectConfig(t *testing.T) {
	res := runCLIInDir(t, http.NotFoundHandler(), t.TempDir(), "rooms", "aliases")
	if res.err == nil || !strings.Contains(res.err.Error(), "chatwork-cli.json") {
		t.Fatalf("error = %v, want mention of missing project config", res.err)
	}
}

func TestTasksListRoomFlagAcceptsAlias(t *testing.T) {
	workDir := t.TempDir()
	writeProjectConfig(t, workDir, `{"rooms":{"main":42}}`)

	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `[]`)
	})
	res := runCLIInDir(t, handler, workDir, "tasks", "list", "--room", "main")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if gotPath != "/rooms/42/tasks" {
		t.Errorf("path = %q, want /rooms/42/tasks", gotPath)
	}
}

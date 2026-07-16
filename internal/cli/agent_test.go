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
)

func newMeServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"account_id":1,"name":"田中"}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func runAgentInit(t *testing.T, workDir, home string, stdin string, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := cli.New(cli.Deps{
		Stdout:     &stdout,
		Stderr:     &stderr,
		Stdin:      strings.NewReader(stdin),
		ConfigPath: filepath.Join(t.TempDir(), "accounts.json"),
		Getenv:     func(string) string { return "" },
		WorkDir:    workDir,
		Home:       home,
	})
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), err
}

func TestAgentInitInstallsRepoSkill(t *testing.T) {
	workDir := t.TempDir()
	stdout, err := runAgentInit(t, workDir, "", "", "agent", "init", "--output", "json")
	if err != nil {
		t.Fatalf("agent init error = %v", err)
	}
	path := filepath.Join(workDir, ".claude", "skills", "chatwork-cli", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("skill not written: %v", err)
	}
	if !strings.Contains(string(data), "name: chatwork-cli") {
		t.Errorf("skill content unexpected:\n%s", string(data)[:100])
	}
	if !strings.Contains(stdout, `"status": "installed"`) {
		t.Errorf("stdout = %s", stdout)
	}

	// 再実行は冪等
	stdout, err = runAgentInit(t, workDir, "", "", "agent", "init", "--output", "json")
	if err != nil {
		t.Fatalf("second run error = %v", err)
	}
	if !strings.Contains(stdout, `"status": "up-to-date"`) {
		t.Errorf("second run stdout = %s", stdout)
	}
}

func TestAgentInitUserScope(t *testing.T) {
	home := t.TempDir()
	_, err := runAgentInit(t, "", home, "", "agent", "init", "--scope", "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "chatwork-cli", "SKILL.md")); err != nil {
		t.Errorf("user-scope skill not written: %v", err)
	}
}

func TestAgentInitRefusesSilentOverwrite(t *testing.T) {
	workDir := t.TempDir()
	path := filepath.Join(workDir, ".claude", "skills", "chatwork-cli", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user-modified content"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runAgentInit(t, workDir, "", "", "agent", "init")
	var usageErr *cli.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("error = %v, want UsageError (non-TTY overwrite without --yes)", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "user-modified content" {
		t.Error("file must not be overwritten without confirmation")
	}

	stdout, err := runAgentInit(t, workDir, "", "", "agent", "init", "--yes", "--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"status": "updated"`) {
		t.Errorf("stdout = %s", stdout)
	}
}

func TestAgentInitInvalidScope(t *testing.T) {
	_, err := runAgentInit(t, t.TempDir(), "", "", "agent", "init", "--scope", "global")
	var usageErr *cli.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("error = %v, want UsageError", err)
	}
}

func TestAgentInitAgentsMDSnippet(t *testing.T) {
	workDir := t.TempDir()
	stdout, err := runAgentInit(t, workDir, "", "", "agent", "init", "--agents-md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"cw docs", "--output json", "cw auth guide"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("snippet missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(workDir, ".claude")); !os.IsNotExist(err) {
		t.Error("--agents-md must not write files")
	}
}

// リポジトリ同梱の SKILL.md はバイナリ埋め込みの正本(internal/cli/SKILL.md)と一致していること。
func TestRepoSkillMatchesEmbedded(t *testing.T) {
	embedded, err := os.ReadFile("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	repoCopy, err := os.ReadFile(filepath.Join("..", "..", ".claude", "skills", "chatwork-cli", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, repoCopy) {
		t.Error("internal/cli/SKILL.md and .claude/skills/chatwork-cli/SKILL.md differ; copy the embedded one over the repo copy")
	}
}

func TestAuthGuideCoversTokenSteps(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{}, "auth", "guide")
	if res.err != nil {
		t.Fatal(res.err)
	}
	for _, want := range []string{"subpackages/api/token.php", "API Token", "cw auth login", "--token-stdin", "CHATWORK_API_TOKEN", "0600", "API利用申請"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("guide missing %q", want)
		}
	}
}

func TestInteractiveLoginShowsTokenGuide(t *testing.T) {
	srv := newMeServer(t)
	var stdout, stderr bytes.Buffer
	root := cli.New(cli.Deps{
		Stdout:     &stdout,
		Stderr:     &stderr,
		Stdin:      strings.NewReader("some-token\n"),
		ConfigPath: filepath.Join(t.TempDir(), "accounts.json"),
		BaseURL:    srv,
		StdoutIsTTY: true,
		StdinIsTTY:  true,
		Getenv:     func(string) string { return "" },
	})
	root.SetArgs([]string{"auth", "login"})
	if err := root.Execute(); err != nil {
		t.Fatalf("error = %v (stderr: %s)", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "subpackages/api/token.php") {
		t.Errorf("interactive login should show the token guide on stderr:\n%s", stderr.String())
	}
}

// TTY では ReadPassword(エコーなし入力)が使われ、Stdin の行読みにフォールバックしないこと。
func TestInteractiveLoginUsesHiddenInput(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("x-chatworktoken")
		fmt.Fprint(w, `{"account_id":1,"name":"田中"}`)
	}))
	t.Cleanup(srv.Close)

	var stdout, stderr bytes.Buffer
	root := cli.New(cli.Deps{
		Stdout:     &stdout,
		Stderr:     &stderr,
		Stdin:      strings.NewReader("must-not-be-read\n"),
		ConfigPath: filepath.Join(t.TempDir(), "accounts.json"),
		BaseURL:    srv.URL,
		StdoutIsTTY: true,
		StdinIsTTY:  true,
		Getenv:     func(string) string { return "" },
		ReadPassword: func() ([]byte, error) {
			return []byte("hidden-token"), nil
		},
	})
	root.SetArgs([]string{"auth", "login"})
	if err := root.Execute(); err != nil {
		t.Fatalf("error = %v (stderr: %s)", err, stderr.String())
	}
	if gotToken != "hidden-token" {
		t.Errorf("validated token = %q, want the one from ReadPassword", gotToken)
	}
}

package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/config"
)

func tempConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "chatwork-cli", "accounts.json")
}

func TestPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("XDG_CONFIG_HOME を優先する", func(t *testing.T) {
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)

		got, err := config.Path()
		if err != nil {
			t.Fatalf("Path() error = %v", err)
		}
		if want := filepath.Join(xdg, "chatwork-cli", "accounts.json"); got != want {
			t.Errorf("Path() = %q, want %q", got, want)
		}
	})

	// 環境変数を継承しないプロセス (launchd / cron / GUI 起動) から呼ばれる経路。
	// ここが ~/.config 以外を指すと、シェルから認証した設定を読めず「未認証」と誤認する。
	t.Run("XDG_CONFIG_HOME が空なら ~/.config を使う", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")

		got, err := config.Path()
		if err != nil {
			t.Fatalf("Path() error = %v", err)
		}
		if want := filepath.Join(home, ".config", "chatwork-cli", "accounts.json"); got != want {
			t.Errorf("Path() = %q, want %q", got, want)
		}
	})
}

func TestLoadMissingFileReturnsEmptyConfig(t *testing.T) {
	cfg, err := config.Load(tempConfigPath(t))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Accounts) != 0 || cfg.Default != "" {
		t.Errorf("Load() = %+v, want empty config", cfg)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := tempConfigPath(t)
	cfg := &config.Config{
		Default: "work",
		Accounts: map[string]config.Account{
			"work": {Token: "tok1", AccountID: 1, Name: "田中"},
			"sub":  {Token: "tok2", AccountID: 2, Name: "個人"},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Default != "work" || len(loaded.Accounts) != 2 {
		t.Errorf("Load() = %+v", loaded)
	}
	if loaded.Accounts["work"].Token != "tok1" {
		t.Errorf("token = %q, want tok1", loaded.Accounts["work"].Token)
	}
}

func TestSaveEnforcesPermissions(t *testing.T) {
	path := tempConfigPath(t)
	cfg := &config.Config{Accounts: map[string]config.Account{"a": {Token: "t"}}}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm = %#o, want 0600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %#o, want 0700", perm)
	}
}

func TestSaveTightensExistingDirPermissions(t *testing.T) {
	path := tempConfigPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Accounts: map[string]config.Account{"a": {Token: "t"}}}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("existing dir perm = %#o, want tightened to 0700", perm)
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	path := tempConfigPath(t)
	cfg := &config.Config{Accounts: map[string]config.Account{"a": {Token: "t"}}}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "accounts.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir entries = %v, want only accounts.json", names)
	}
}

func TestPermWarning(t *testing.T) {
	path := tempConfigPath(t)
	cfg := &config.Config{Accounts: map[string]config.Account{}}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	if warn := config.PermWarning(path); warn != "" {
		t.Errorf("PermWarning() on 0600 = %q, want empty", warn)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if warn := config.PermWarning(path); !strings.Contains(warn, "loose permissions") {
		t.Errorf("PermWarning() on 0644 = %q, want loose permissions warning", warn)
	}
}

func TestResolve(t *testing.T) {
	two := map[string]config.Account{
		"work": {Token: "tok1"},
		"sub":  {Token: "tok2"},
	}
	tests := []struct {
		name      string
		cfg       *config.Config
		alias     string
		wantName  string
		wantErrIn string
	}{
		{"explicit alias", &config.Config{Accounts: two}, "sub", "sub", ""},
		{"unknown alias", &config.Config{Accounts: two}, "nope", "", "not found"},
		{"default", &config.Config{Default: "work", Accounts: two}, "", "work", ""},
		{"single account without default", &config.Config{Accounts: map[string]config.Account{"only": {Token: "t"}}}, "", "only", ""},
		{"no accounts", &config.Config{Accounts: map[string]config.Account{}}, "", "", "no accounts"},
		{"ambiguous", &config.Config{Accounts: two}, "", "", "multiple accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, _, err := tt.cfg.Resolve(tt.alias)
			if tt.wantErrIn != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("Resolve() error = %v, want containing %q", err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if name != tt.wantName {
				t.Errorf("Resolve() name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

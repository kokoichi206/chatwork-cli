// Package config は複数アカウントの認証情報を accounts.json として読み書きする。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Account struct {
	Token     string `json:"token"`
	AccountID int    `json:"account_id"`
	Name      string `json:"name"`
}

type Config struct {
	Default  string             `json:"default,omitempty"`
	Accounts map[string]Account `json:"accounts"`
}

// Path は accounts.json の保存先を返す。
// XDG_CONFIG_HOME を最優先することで、macOS でも ~/.config 配下に設定を統一できる。
func Path() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "chatwork-cli", "accounts.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "chatwork-cli", "accounts.json"), nil
}

// Load はファイルが存在しない場合、空の Config を返す(初回起動を正常系として扱う)。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{Accounts: map[string]Account{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Accounts == nil {
		cfg.Accounts = map[string]Account{}
	}
	return &cfg, nil
}

// Save はトークンを含むため、ディレクトリ 0700・ファイル 0600 で書き込む。
// 全アカウントのトークンを 1 ファイルに持つため、書き込み途中のクラッシュで
// 既存内容が破損しないよう一時ファイル + rename でアトミックに置き換える。
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// 既存ディレクトリには MkdirAll の perm が適用されないため明示的に絞る。
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".accounts-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // rename 成功後は ENOENT で無害
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// PermWarning はファイルの権限が 0600 より緩い場合に警告文を返す。問題なければ空文字。
func PermWarning(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Sprintf("warning: %s has loose permissions %#o; run: chmod 600 %s", path, perm, path)
	}
	return ""
}

// Resolve はアカウントを選択する。優先順: alias 指定 > default > 唯一のアカウント。
func (c *Config) Resolve(alias string) (string, Account, error) {
	if alias != "" {
		acct, ok := c.Accounts[alias]
		if !ok {
			return "", Account{}, fmt.Errorf("account %q not found; run `cw auth list`", alias)
		}
		return alias, acct, nil
	}
	if c.Default != "" {
		acct, ok := c.Accounts[c.Default]
		if !ok {
			return "", Account{}, fmt.Errorf("default account %q not found; run `cw auth set-default`", c.Default)
		}
		return c.Default, acct, nil
	}
	if len(c.Accounts) == 1 {
		for name, acct := range c.Accounts {
			return name, acct, nil
		}
	}
	if len(c.Accounts) == 0 {
		return "", Account{}, errors.New("no accounts configured; run `cw auth login` (see `cw auth guide` for how to get a token)")
	}
	return "", Account{}, fmt.Errorf("multiple accounts (%s); pass --account or run `cw auth set-default`", joinNames(c.Accounts))
}

func joinNames(accounts map[string]Account) string {
	names := make([]string, 0, len(accounts))
	for name := range accounts {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

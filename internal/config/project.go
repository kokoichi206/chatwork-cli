package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Project はリポジトリスコープの設定(.config/chatwork-cli.json)。
// チーム共有前提でコミットされるため、個人差のある値(auth の alias 名・トークン・
// アカウント種別に依存する情報)は持たず、部屋のエイリアスだけを定義する。
type Project struct {
	Rooms map[string]int `json:"rooms,omitempty"`

	// Dir は設定ファイルが見つかったディレクトリ(リポジトリルート相当)。
	Dir string `json:"-"`
}

// ProjectConfigName は https://github.com/pi0/config-dir の規約に従う:
// .config/ 配下、ツール名を明示、.config サフィックスは付けない。
const ProjectConfigName = "chatwork-cli.json"

// FindProject は startDir から親方向に .config/chatwork-cli.json を探す。
// stopDir(通常は HOME)に到達した時点で打ち切る(ユーザースコープの設定と混同しないため)。
// 見つからない場合は (nil, nil)。
func FindProject(startDir, stopDir string) (*Project, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	if stopDir != "" {
		if abs, err := filepath.Abs(stopDir); err == nil {
			stopDir = abs
		}
	}
	for {
		if stopDir != "" && dir == stopDir {
			return nil, nil
		}
		path := filepath.Join(dir, ".config", ProjectConfigName)
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			var p Project
			if err := json.Unmarshal(data, &p); err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			p.Dir = dir
			return &p, nil
		case !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// RoomAliases はエイリアス名をソートして返す(エラーメッセージ・一覧表示用)。
func (p *Project) RoomAliases() []string {
	aliases := make([]string, 0, len(p.Rooms))
	for name := range p.Rooms {
		aliases = append(aliases, name)
	}
	sort.Strings(aliases)
	return aliases
}

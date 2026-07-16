// Package store は Chatwork メッセージのローカル履歴を JSONL で蓄積する。
//
// ローカル蓄積が必要な理由(Chatwork API v2 の契約上の制約):
//   - GET /rooms/{room_id}/messages は最新 100 件のみを返し、過去へ遡るページネーションが存在しない
//     (https://developer.chatwork.com/reference/get-rooms-room_id-messages)。
//     100 件より古いメッセージを API から取得する手段はないため、履歴は取得時点でローカルに残すしかない。
//   - force=0 の差分カーソルはサーバー側に保持される共有状態で、agent・人間・他ツールが
//     同じカーソルを消費し合うと「どこまで読んだか」が壊れる。カーソルの管理単位
//     (トークンかアカウントか)は公式ドキュメントに明記されていないが、どちらでも共有で壊れる点は同じ。
//     このため差分判定はサーバーカーソルに依存せず、ローカル履歴との message_id 突き合わせで行う。
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kokoichi206/chatwork-cli/internal/chatwork"
)

// Dir はローカル履歴のルートディレクトリを返す。
// 設定(~/.config)とデータ(~/.local/share)を分ける XDG の役割分担に従う。
func Dir(getenv func(string) string, home string) string {
	if xdg := getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "chatwork-cli")
	}
	return filepath.Join(home, ".local", "share", "chatwork-cli")
}

// RoomPath は部屋ごとの履歴ファイルのパスを返す。
// account_id をパスに含めるのは、複数アカウントが同じ部屋に入っていても履歴を混ぜないため。
func RoomPath(dataDir string, accountID, roomID int) string {
	return filepath.Join(dataDir, strconv.Itoa(accountID), "rooms", fmt.Sprintf("%d.jsonl", roomID))
}

// Load は履歴を古い順で返す。ファイルがなければ空を返す(初回同期を正常系として扱う)。
func Load(path string) ([]chatwork.Message, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []chatwork.Message{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // 読み取り専用のため Close エラーは結果に影響しない

	msgs := []chatwork.Message{}
	sc := bufio.NewScanner(f)
	// メッセージ本文は Scanner 既定の 64KiB を超えうるため上限を広げる。
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		if len(sc.Bytes()) == 0 {
			continue
		}
		var m chatwork.Message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, err)
		}
		msgs = append(msgs, m)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return msgs, nil
}

// apiWindow は messages API が一度に返す最大件数。
// 同期間隔中の投稿がこれを超えると、溢れた分は API から永久に取得できない。
const apiWindow = 100

// SyncResult は Merge の差分内訳。New は今回初めて現れたメッセージ(取得順)。
type SyncResult struct {
	New     []chatwork.Message
	Updated int
	Gap     bool
}

// Merge は API から取得した最新分(古い順)をローカル履歴へ統合し、
// 保存対象の全履歴と差分内訳を返す。message_id を主キーに dedupe し、
// 既存 message_id で内容が変わっていれば編集とみなして上書きする。
//
// Gap は「取得が apiWindow 件ちょうど、かつローカル最新の message_id がその中にない」
// ことで検知する。取得が apiWindow 件未満なら部屋の全メッセージが窓に収まっており
// 溢れは起こりえない(その場合のローカル最新の不在は削除を意味するので gap にしない)。
// apiWindow 件ちょうどのときは溢れと削除を区別できないため gap は「溢れの可能性」を示し、
// 溢れていた場合に埋める手段はない。
func Merge(local, fetched []chatwork.Message) ([]chatwork.Message, SyncResult) {
	index := make(map[string]int, len(local))
	for i, m := range local {
		index[m.MessageID] = i
	}

	merged := make([]chatwork.Message, len(local), len(local)+len(fetched))
	copy(merged, local)
	res := SyncResult{New: []chatwork.Message{}}

	fetchedIDs := make(map[string]bool, len(fetched))
	for _, m := range fetched {
		fetchedIDs[m.MessageID] = true
		if i, ok := index[m.MessageID]; ok {
			if merged[i].UpdateTime != m.UpdateTime || merged[i].Body != m.Body {
				merged[i] = m
				res.Updated++
			}
			continue
		}
		index[m.MessageID] = len(merged)
		merged = append(merged, m)
		res.New = append(res.New, m)
	}

	if len(local) > 0 && len(fetched) == apiWindow && !fetchedIDs[local[len(local)-1].MessageID] {
		res.Gap = true
	}
	return merged, res
}

// Save は履歴全体(古い順)を 1 行 1 メッセージの JSONL として書き込む。
// メッセージ本文は非公開情報のためディレクトリ 0700・ファイル 0600 とし、
// 書き込み途中のクラッシュで既存履歴が壊れないよう一時ファイル + rename で置き換える。
func Save(path string, msgs []chatwork.Message) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sync-*.jsonl")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // rename 成功後は ENOENT で無害
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

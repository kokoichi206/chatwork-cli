package store_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/chatwork"
	"github.com/kokoichi206/chatwork-cli/internal/store"
)

func msg(id string, body string) chatwork.Message {
	n, _ := strconv.ParseInt(id, 10, 64)
	return chatwork.Message{MessageID: id, Body: body, SendTime: n, UpdateTime: 0}
}

func ids(msgs []chatwork.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.MessageID
	}
	return out
}

func equalIDs(a []chatwork.Message, want ...string) bool {
	got := ids(a)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestMergeFirstSyncTakesAllFetched(t *testing.T) {
	fetched := []chatwork.Message{msg("1", "a"), msg("2", "b")}
	merged, res := store.Merge(nil, fetched)
	if !equalIDs(merged, "1", "2") {
		t.Errorf("merged = %v", ids(merged))
	}
	if len(res.New) != 2 || res.Updated != 0 || res.Gap {
		t.Errorf("res = %+v", res)
	}
}

func TestMergeDedupesOverlappingWindow(t *testing.T) {
	local := []chatwork.Message{msg("1", "a"), msg("2", "b")}
	fetched := []chatwork.Message{msg("2", "b"), msg("3", "c")}
	merged, res := store.Merge(local, fetched)
	if !equalIDs(merged, "1", "2", "3") {
		t.Errorf("merged = %v", ids(merged))
	}
	if !equalIDs(res.New, "3") || res.Updated != 0 || res.Gap {
		t.Errorf("res = %+v", res)
	}
}

func TestMergeOverwritesEditedMessage(t *testing.T) {
	local := []chatwork.Message{msg("1", "a"), msg("2", "old")}
	edited := msg("2", "edited")
	edited.UpdateTime = 100
	merged, res := store.Merge(local, []chatwork.Message{edited, msg("3", "c")})
	if merged[1].Body != "edited" {
		t.Errorf("edited message not overwritten: %+v", merged[1])
	}
	if res.Updated != 1 || !equalIDs(res.New, "3") {
		t.Errorf("res = %+v", res)
	}
}

// fullWindow は messages API の 1 回分(100 件)に相当する取得結果を生成する。
func fullWindow(startID int) []chatwork.Message {
	msgs := make([]chatwork.Message, 100)
	for i := range msgs {
		msgs[i] = msg(strconv.Itoa(startID+i), "m")
	}
	return msgs
}

func TestMergeDetectsGap(t *testing.T) {
	local := []chatwork.Message{msg("1", "a"), msg("2", "b")}
	// 取得が窓いっぱい(100 件)で、ローカル最新(2)がその中にない → 溢れの可能性
	merged, res := store.Merge(local, fullWindow(200))
	if !res.Gap {
		t.Error("gap not detected")
	}
	if len(merged) != 102 || merged[0].MessageID != "1" || merged[101].MessageID != "299" {
		t.Errorf("merged len=%d ids=%v", len(merged), ids(merged)[:3])
	}
}

func TestMergeNoGapWhenWindowNotFull(t *testing.T) {
	local := []chatwork.Message{msg("1", "a"), msg("2", "b")}
	// 取得が 100 件未満なら部屋の全メッセージが窓内 → ローカル最新の不在は削除であり gap ではない
	fetched := []chatwork.Message{msg("1", "a"), msg("3", "c")}
	_, res := store.Merge(local, fetched)
	if res.Gap {
		t.Error("gap must be false when the fetch window is not full")
	}
}

func TestMergeNoGapWhenFetchedEmpty(t *testing.T) {
	local := []chatwork.Message{msg("1", "a")}
	_, res := store.Merge(local, nil)
	if res.Gap {
		t.Error("gap must be false for empty fetch")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := store.RoomPath(t.TempDir(), 100, 42)
	msgs := []chatwork.Message{
		msg("1", "改行\nあり"),
		msg("2", `"quote" と <tag>`),
	}
	if err := store.Save(path, msgs); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Body != msgs[0].Body || got[1].Body != msgs[1].Body {
		t.Errorf("got = %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %#o, want 0600", perm)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	got, err := store.Load(filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v", got)
	}
}

func TestDirRespectsXDGDataHome(t *testing.T) {
	getenv := func(key string) string {
		if key == "XDG_DATA_HOME" {
			return "/xdg/data"
		}
		return ""
	}
	if got := store.Dir(getenv, "/home/u"); got != "/xdg/data/chatwork-cli" {
		t.Errorf("Dir = %q", got)
	}
	if got := store.Dir(func(string) string { return "" }, "/home/u"); got != filepath.Join("/home/u", ".local", "share", "chatwork-cli") {
		t.Errorf("Dir = %q", got)
	}
}

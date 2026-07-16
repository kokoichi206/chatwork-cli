package store

import (
	"os"
	"path/filepath"
)

// Lock は path に対応するロックファイルで排他ロックを取得し、解放関数を返す。
// sync の Load → Merge → Save は read-modify-write であり、同じ部屋への並行 sync を
// 直列化しないと、後から Save した側が先行プロセスの新着を上書きして履歴を失う
// (Save 単体のアトミック rename では防げない)。先行プロセスが解放するまでブロックする。
func Lock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flockExclusive(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = funlock(f)
		_ = f.Close()
	}, nil
}

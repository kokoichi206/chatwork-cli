// Package version はビルド時に埋め込まれるバージョン情報を保持する。
package version

import (
	"fmt"
	"runtime"
)

// 以下の変数は goreleaser の ldflags で上書きされる。
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String はバージョン情報を 1 行で返す。
// cobra の version テンプレート「cw version {{.Version}}」に埋め込まれるため、
// 先頭に "cw" や "version" を含めず、"version " に続けて自然に読める形にする。
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s)", Version, Commit, Date, runtime.Version())
}

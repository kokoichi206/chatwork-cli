// Package output は CLI の出力整形を担う。
// JSON はスキーマ安定性を優先し、API レスポンスの型をそのまま marshal する。
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type Format string

const (
	FormatJSON  Format = "json"
	FormatTable Format = "table"
	FormatText  Format = "text"
)

func Parse(s string) (Format, error) {
	switch Format(s) {
	case FormatJSON, FormatTable, FormatText:
		return Format(s), nil
	case "":
		return "", fmt.Errorf("output format is empty")
	default:
		return "", fmt.Errorf("unknown output format %q (json|table|text)", s)
	}
}

// Default は TTY では人間向けの table、パイプ時は機械可読な json を返す。
// agent がフラグを付け忘れてもパイプ経由なら JSON が得られるようにするための既定。
func Default(isTTY bool) Format {
	if isTTY {
		return FormatTable
	}
	return FormatJSON
}

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func WriteTable(w io.Writer, header []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, row := range rows {
		for i, cell := range row {
			row[i] = sanitizeCell(cell)
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}

// sanitizeCell は改行・タブを潰して 1 セル 1 行に収める(本文プレビュー用)。
func sanitizeCell(s string) string {
	s = StripControl(s)
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// StripControl は改行・タブ以外の制御文字を除去する。
// Chatwork のメッセージ本文・表示名は他の参加者が自由に書けるため、
// ANSI/OSC エスケープをそのまま端末へ流すと表示偽装等の注入が成立する。
// table/text 出力の前に必ず通すこと(JSON 出力はエンコーダがエスケープするため対象外)。
func StripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// Truncate はテーブル表示用に rune 単位で切り詰める。
func Truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

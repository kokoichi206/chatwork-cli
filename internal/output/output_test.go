package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/output"
)

func TestParse(t *testing.T) {
	for _, valid := range []string{"json", "table", "text"} {
		if _, err := output.Parse(valid); err != nil {
			t.Errorf("Parse(%q) error = %v", valid, err)
		}
	}
	if _, err := output.Parse("yaml"); err == nil {
		t.Error("Parse(yaml) should fail")
	}
}

func TestDefault(t *testing.T) {
	if got := output.Default(true); got != output.FormatTable {
		t.Errorf("Default(TTY) = %v, want table", got)
	}
	if got := output.Default(false); got != output.FormatJSON {
		t.Errorf("Default(pipe) = %v, want json", got)
	}
}

func TestWriteJSONDoesNotEscapeHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]string{"body": "[To:1] <ok> & done"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "<ok>") || strings.Contains(out, "\\u003c") {
		t.Errorf("HTML-escaped output: %s", out)
	}
}

func TestWriteTableFlattensNewlines(t *testing.T) {
	var buf bytes.Buffer
	err := output.WriteTable(&buf, []string{"ID", "BODY"}, [][]string{{"1", "line1\nline2"}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("table rows must stay on one line each:\n%s", buf.String())
	}
}

func TestStripControlRemovesEscapeSequences(t *testing.T) {
	got := output.StripControl("safe\x1b[31mred\x1b]0;title\x07\r\x00 text\nline\ttab")
	want := "safe[31mred]0;title text\nline\ttab"
	if got != want {
		t.Errorf("StripControl() = %q, want %q", got, want)
	}
}

func TestWriteTableStripsControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteTable(&buf, []string{"BODY"}, [][]string{{"a\x1b[2Jb"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("table output contains ESC: %q", buf.String())
	}
}

func TestTruncate(t *testing.T) {
	if got := output.Truncate("こんにちは世界", 5); got != "こんにちは…" {
		t.Errorf("Truncate() = %q", got)
	}
	if got := output.Truncate("short", 10); got != "short" {
		t.Errorf("Truncate() = %q", got)
	}
}

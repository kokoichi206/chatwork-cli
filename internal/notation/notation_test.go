package notation_test

import (
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/notation"
)

func TestMention(t *testing.T) {
	if got, want := notation.Mention(123456), "[To:123456]"; got != want {
		t.Errorf("Mention() = %q, want %q", got, want)
	}
}

func TestReplyTag(t *testing.T) {
	got := notation.ReplyTag(123, 45678, "1234567890123456789")
	want := "[rp aid=123 to=45678-1234567890123456789]"
	if got != want {
		t.Errorf("ReplyTag() = %q, want %q", got, want)
	}
}

func TestQuote(t *testing.T) {
	got := notation.Quote(123, 1700000000, "hello")
	want := "[qt][qtmeta aid=123 time=1700000000]hello[/qt]"
	if got != want {
		t.Errorf("Quote() = %q, want %q", got, want)
	}
}

func TestInfo(t *testing.T) {
	tests := []struct {
		name  string
		title string
		body  string
		want  string
	}{
		{"without title", "", "body", "[info]body[/info]"},
		{"with title", "件名", "body", "[info][title]件名[/title]body[/info]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notation.Info(tt.title, tt.body); got != tt.want {
				t.Errorf("Info() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplyBody(t *testing.T) {
	tests := []struct {
		name        string
		withMention bool
		want        string
	}{
		{
			name:        "mention notifies the author",
			withMention: true,
			want:        "[rp aid=123 to=456-789][To:123]\nありがとうございます",
		},
		{
			name:        "no-mention keeps only the RE link",
			withMention: false,
			want:        "[rp aid=123 to=456-789]\nありがとうございます",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notation.ReplyBody(123, 456, "789", "ありがとうございます", tt.withMention)
			if got != tt.want {
				t.Errorf("ReplyBody() = %q, want %q", got, tt.want)
			}
		})
	}
}
